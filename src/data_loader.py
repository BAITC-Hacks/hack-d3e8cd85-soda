"""Загрузка, очистка, нормализация дат и происхождения профилей."""

from __future__ import annotations

import datetime
import importlib
import json
import logging
import re
from pathlib import Path
from typing import Any
from uuid import uuid4

logger = logging.getLogger(__name__)
REQUIRED_FIELDS = {"id", "anon_name", "categories", "city", "price_from_kzt", "event_formats", "languages", "max_hours", "busy_dates", "description", "synthetic", "city_imputed", "price_imputed"}


def _bool(value: Any) -> bool:
    if isinstance(value, str):
        return value.strip().lower() in {"true", "1", "yes", "да"}
    return bool(value)


def _list(value: Any) -> list[str]:
    if value is None:
        return []
    values = value if isinstance(value, list) else value.split("|") if isinstance(value, str) else [value]
    return [str(item).strip() for item in values if str(item).strip()]


def _normalize_record(record: dict[str, Any]) -> dict[str, Any]:
    result = dict(record)
    for field in ("categories", "event_formats", "languages", "busy_dates"):
        result[field] = _list(record.get(field))
    max_hours = record.get("max_hours")
    try:
        result["max_hours"] = None if max_hours in (None, "") else int(max_hours)
    except (TypeError, ValueError):
        result["max_hours"] = None
    for field in ("synthetic", "city_imputed", "price_imputed"):
        result[field] = _bool(record.get(field, False))
    return result


def load_dataset(file_path: str) -> list[dict]:
    """Загружает JSONL или JSON-массив, пропуская повреждённые записи."""
    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"Dataset file not found: {path}")
    text = path.read_text(encoding="utf-8")
    records: list[Any] = []
    if text.lstrip().startswith("["):
        try:
            parsed = json.loads(text)
            records = parsed if isinstance(parsed, list) else []
        except json.JSONDecodeError as exc:
            logger.error("Некорректный JSON-массив: %s", exc)
    else:
        for line_number, line in enumerate(text.splitlines(), 1):
            if not line.strip():
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError as exc:
                logger.warning("Пропуск строки %d: %s", line_number, exc)
    result: list[dict] = []
    for index, record in enumerate(records, 1):
        if not isinstance(record, dict):
            logger.warning("Пропуск элемента %d: ожидался объект", index)
            continue
        missing = REQUIRED_FIELDS - record.keys()
        if missing:
            logger.warning("Пропуск элемента %d: отсутствуют %s", index, ", ".join(sorted(missing)))
            continue
        result.append(_normalize_record(record))
    logger.info("Загружено записей: %d", len(result))
    return result


def get_dataset_summary(data: list[dict]) -> dict:
    """Возвращает базовую статистику каталога."""
    return {
        "total_records": len(data),
        "real_profiles": sum(not _bool(item.get("synthetic", False)) for item in data),
        "synthetic_profiles": sum(_bool(item.get("synthetic", False)) for item in data),
        "unique_cities": sorted({str(item.get("city", "")).strip() for item in data if item.get("city")}),
        "unique_categories": sorted({str(category).strip() for item in data for category in _list(item.get("categories"))}),
    }


def clean_text(text: object) -> str:
    """Очищает пробелы и приводит текст к нижнему регистру."""
    return " ".join(text.strip().lower().split()) if isinstance(text, str) else ""


_SYNONYMS = {
    "city": {"алмату": "алматы", "астаны": "астана", "нур султан": "астана", "за рубежом": "зарубежье"},
    "categories": {"фотограф": "фотография", "тамада": "ведущий", "видеограф": "видеография"},
    "event_formats": {"свадьбы": "свадьба", "тои": "той", "корпоративы": "корпоратив"},
    "languages": {"ru": "русский", "рус": "русский", "kz": "казахский", "kaz": "казахский", "en": "английский", "eng": "английский", "english": "английский"},
}


def _value(value: Any, field: str) -> str:
    value = re.sub(r"^[^\w]+|[^\w]+$", "", clean_text(value), flags=re.UNICODE)
    synonym = _SYNONYMS.get(field, {}).get(value)
    return synonym if synonym is not None else value


def _values(value: Any, field: str) -> list[str]:
    raw = _list(value)
    result: list[str] = []
    for item in raw:
        normalized = _value(item, field)
        if normalized and normalized not in result:
            result.append(normalized)
    return result


def normalize_profile_fields(profile: dict) -> dict:
    """Возвращает копию профиля с каноническими текстовыми полями."""
    result = dict(profile)
    if "city" in profile:
        result["city_original"] = profile["city"]
        result["city"] = _value(profile["city"], "city")
    for field in ("categories", "event_formats", "languages"):
        if field in profile:
            result[f"{field}_original"] = profile[field]
            result[field] = _values(profile[field], field)
    return result


def normalize_user_query(query: dict) -> dict:
    """Нормализует параметры запроса, сохраняя compatibility-ключи."""
    result = dict(query)
    if "city" in query:
        result["city"] = _value(query["city"], "city")
    category = query.get("categories", query.get("category"))
    event_format = query.get("event_formats", query.get("event_format"))
    if category is not None:
        result["categories"] = _values(category, "categories")
    if event_format is not None:
        result["event_formats"] = _values(event_format, "event_formats")
    if "languages" in query or "language" in query:
        result["languages"] = _values(query.get("languages", query.get("language")), "languages")
    return result


def parse_date(date_str: str) -> datetime.date | None:
    """Разбирает поддерживаемые форматы даты."""
    if not isinstance(date_str, str):
        return None
    for fmt in ("%Y-%m-%d", "%d.%m.%Y", "%Y/%m/%d", "%d/%m/%Y", "%Y:%m:%d", "%d:%m:%Y"):
        try:
            return datetime.datetime.strptime(date_str.strip(), fmt).date()
        except ValueError:
            continue
    logger.warning("Не удалось разобрать дату %r", date_str)
    return None


def normalize_dataset_dates(data: list[dict]) -> list[dict]:
    """Добавляет к профилям множество ``busy_dates_parsed``."""
    result = []
    for profile in data:
        copy = dict(profile)
        copy["busy_dates_parsed"] = {parsed for item in _list(profile.get("busy_dates")) if (parsed := parse_date(item)) is not None}
        result.append(copy)
    return result


def is_date_in_range(target_date: datetime.date, start_str: str = "2026-09-23", end_str: str = "2026-12-31") -> bool:
    """Проверяет включительный диапазон календаря проекта."""
    start, end = parse_date(start_str), parse_date(end_str)
    return isinstance(target_date, datetime.date) and not isinstance(target_date, datetime.datetime) and start is not None and end is not None and start <= target_date <= end


def process_synthetic_flags(profile: dict) -> dict:
    """Гарантирует synthetic и badge для UI."""
    result = dict(profile)
    result["synthetic"] = _bool(result.get("synthetic", False))
    result["badge"] = "[Синтетический профиль]" if result["synthetic"] else None
    return result


def create_synthetic_profile(anon_name: str, categories: list[str], city: str, price_from_kzt: int | float, event_formats: list[str], languages: list[str], max_hours: int | None, busy_dates: list[str], description: str) -> dict:
    """Создаёт минимально валидный синтетический профиль."""
    if not anon_name.strip() or not city.strip() or price_from_kzt <= 0:
        raise ValueError("anon_name/city должны быть непустыми, цена должна быть больше 0")
    return process_synthetic_flags({"id": f"synthetic-{uuid4().hex}", "anon_name": anon_name, "categories": categories, "city": city, "price_from_kzt": price_from_kzt, "event_formats": event_formats, "languages": languages, "max_hours": max_hours, "busy_dates": busy_dates, "description": description, "synthetic": True, "city_imputed": False, "price_imputed": False})


def load_dataset_pandas(file_path: str) -> Any:
    """Опционально загружает JSONL через pandas для EDA."""
    pandas = importlib.import_module("pandas")
    return pandas.read_json(file_path, lines=True, encoding="utf-8")
