"""Жёсткие фильтры по городу, календарю, бюджету и опциям."""

from __future__ import annotations

import datetime
import logging
import math
from numbers import Real
from typing import Any

from .data_loader import clean_text, is_date_in_range, parse_date

logger = logging.getLogger(__name__)


def _categories(profile: dict) -> list[str]:
    value = profile.get("categories")
    return value if isinstance(value, list) else [value] if isinstance(value, str) else []


def filter_by_city_and_category(dataset: list[dict], target_city: str, target_category: str) -> list[dict]:
    """Оставляет профили с полным совпадением города и категории."""
    city, category = clean_text(target_city), clean_text(target_category)
    if not city or not category:
        return []
    return [profile for profile in dataset if clean_text(profile.get("city")) == city and any(clean_text(item) == category for item in _categories(profile))]


def check_category_exists_in_city(dataset: list[dict], target_city: str, target_category: str) -> bool:
    """Проверяет наличие категории в городе."""
    return bool(filter_by_city_and_category(dataset, target_city, target_category))


def _busy_dates(profile: dict) -> set[datetime.date]:
    parsed = profile.get("busy_dates_parsed")
    if isinstance(parsed, set):
        return {item for item in parsed if isinstance(item, datetime.date) and not isinstance(item, datetime.datetime)}
    values = profile.get("busy_dates") or []
    values = [values] if isinstance(values, str) else values if isinstance(values, (list, tuple, set)) else []
    return {date for value in values if isinstance(value, str) and (date := parse_date(value)) is not None}


def filter_by_availability(dataset: list[dict], target_date: datetime.date) -> tuple[list[dict], list[dict]]:
    """Разделяет профили по занятости; дата обязана быть в календаре проекта."""
    if isinstance(target_date, datetime.datetime) or not isinstance(target_date, datetime.date):
        raise TypeError("target_date должен быть datetime.date")
    if not is_date_in_range(target_date):
        raise ValueError("Дата вне диапазона 2026-09-23...2026-12-31")
    available, busy = [], []
    for profile in dataset:
        (busy if target_date in _busy_dates(profile) else available).append(profile)
    return available, busy


def get_occupancy_stats(dataset: list[dict], target_date: datetime.date) -> dict:
    """Возвращает число и процент занятых профилей."""
    available, busy = filter_by_availability(dataset, target_date)
    return {"total_contractors": len(dataset), "available_count": len(available), "busy_count": len(busy), "occupancy_percent": len(busy) / len(dataset) * 100 if dataset else 0.0}


def _price(profile: dict) -> float | None:
    value = profile.get("price_from_kzt")
    if isinstance(value, bool) or not isinstance(value, Real):
        return None
    value = float(value)
    return value if math.isfinite(value) and value > 0 else None


def filter_by_budget(dataset: list[dict], budget: float) -> tuple[list[dict], list[dict]]:
    """Разделяет профили на доступные и превышающие бюджет."""
    if isinstance(budget, bool) or not isinstance(budget, Real) or not math.isfinite(float(budget)) or budget <= 0:
        raise ValueError("Бюджет должен быть положительным конечным числом")
    affordable, expensive = [], []
    for profile in dataset:
        price = _price(profile)
        if price is None:
            logger.warning("Профиль %r пропущен: некорректная цена", profile.get("id"))
        elif price <= float(budget):
            affordable.append(profile)
        else:
            expensive.append(profile)
    return affordable, expensive


def analyze_budget_fit(dataset: list[dict], budget: float) -> dict:
    """Готовит минимум цены и статистику отсева по бюджету."""
    _, expensive = filter_by_budget(dataset, budget)
    prices = [price for profile in dataset if (price := _price(profile)) is not None]
    minimum = min(prices, default=None)
    return {"min_price_in_category": minimum, "excluded_by_budget_count": len(expensive), "budget_shortfall": max(0.0, minimum - float(budget)) if minimum is not None else None}


def _values(value: Any) -> set[str]:
    values = [value] if isinstance(value, str) else value if isinstance(value, (list, tuple, set)) else []
    return {clean_text(item) for item in values if clean_text(item)}


def filter_by_optional_criteria(dataset: list[dict], event_format: str | None = None, language: str | None = None, duration_hours: float | None = None) -> tuple[list[dict], dict[str, int]]:
    """Фильтрует по формату, языку и длительности."""
    if duration_hours is not None and (isinstance(duration_hours, bool) or not isinstance(duration_hours, Real) or not math.isfinite(float(duration_hours)) or duration_hours < 0):
        raise ValueError("duration_hours должен быть неотрицательным конечным числом")
    requested_format, requested_language = clean_text(event_format), clean_text(language)
    reasons = {"event_format": 0, "language": 0, "duration_hours": 0}
    result = []
    for profile in dataset:
        format_ok = not requested_format or requested_format in _values(profile.get("event_formats"))
        language_ok = not requested_language or requested_language in _values(profile.get("languages"))
        max_hours = profile.get("max_hours")
        duration_ok = duration_hours is None or max_hours is None or isinstance(max_hours, (int, float)) and not isinstance(max_hours, bool) and max_hours >= duration_hours
        reasons["event_format"] += not format_ok
        reasons["language"] += not language_ok
        reasons["duration_hours"] += not duration_ok
        if format_ok and language_ok and duration_ok:
            result.append(profile)
    return result, reasons


def run_hard_filters(dataset: list[dict], target_city: str, target_category: str, event_date: datetime.date, budget: float, event_format: str | None = None, language: str | None = None, duration_hours: float | None = None) -> tuple[list[dict], dict]:
    """Последовательно применяет все hard-фильтры."""
    city_matches = filter_by_city_and_category(dataset, target_city, target_category)
    available, busy = filter_by_availability(city_matches, event_date)
    affordable, expensive = filter_by_budget(available, budget)
    result, optional_reasons = filter_by_optional_criteria(affordable, event_format, language, duration_hours)
    return result, {"total_input": len(dataset), "after_city_category": len(city_matches), "after_availability": len(available), "after_budget": len(affordable), "after_optional_criteria": len(result), "rejected_by_step": {"city_category": len(dataset) - len(city_matches), "availability": len(busy), "budget": len(expensive), "optional_criteria": len(affordable) - len(result)}, "optional_rejection_reasons": optional_reasons}
