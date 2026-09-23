"""Исходы поиска, объяснения, LLM prompt и финальные карточки."""

from __future__ import annotations

import datetime
import json
import re
from dataclasses import dataclass, field
from enum import Enum
from numbers import Real
from typing import Any

from .data_loader import clean_text


class SearchOutcome(str, Enum):
    MATCH_FOUND = "MATCH_FOUND"
    NO_CATEGORY_IN_CITY = "NO_CATEGORY_IN_CITY"
    NO_MATCHES_AFTER_FILTERS = "NO_MATCHES_AFTER_FILTERS"


@dataclass
class SearchResult:
    """Единая модель ответа поиска."""
    outcome: SearchOutcome | str
    cards: list[dict[str, Any]] = field(default_factory=list)
    message: str = ""
    details: dict[str, Any] = field(default_factory=dict)

    def as_dict(self) -> dict[str, Any]:
        return {"outcome": self.outcome.value if isinstance(self.outcome, SearchOutcome) else self.outcome, "cards": self.cards, "message": self.message, "details": self.details}


def _categories(profile: dict) -> list[str]:
    value = profile.get("categories")
    return value if isinstance(value, list) else [value] if isinstance(value, str) else []


def check_category_in_city_outcome(dataset: list[dict], target_city: str, target_category: str) -> SearchResult | None:
    """Возвращает Исход B до дорогих фильтров или None при наличии категории."""
    city, category = clean_text(target_city), clean_text(target_category)
    if any(clean_text(item.get("city")) == city and any(clean_text(x) == category for x in _categories(item)) for item in dataset):
        return None
    alternatives = get_available_categories_for_city(dataset, city)
    message = f"В городе {target_city} отсутствуют подрядчики категории {target_category}."
    if alternatives:
        message += " Доступные категории: " + ", ".join(alternatives) + "."
    return SearchResult(SearchOutcome.NO_CATEGORY_IN_CITY, [], message, {"target_city": city, "target_category": category, "available_categories": alternatives})


def get_available_categories_for_city(dataset: list[dict], target_city: str) -> list[str]:
    """Возвращает уникальные категории города."""
    city = clean_text(target_city)
    return sorted({clean_text(category) for item in dataset if clean_text(item.get("city")) == city for category in _categories(item) if clean_text(category)})


def _count(stats: dict, name: str) -> int:
    values = stats.get("rejected_by_step", stats)
    value = values.get(name, 0) if isinstance(values, dict) else 0
    return value if isinstance(value, int) and value > 0 else 0


def _date(value: Any) -> str:
    if isinstance(value, datetime.datetime):
        value = value.date()
    return value.strftime("%d.%m.%Y") if isinstance(value, datetime.date) else str(value or "выбранную дату")


def analyze_no_matches_outcome(city_category_pool: list[dict], user_query: dict, filter_pipeline_results: dict) -> SearchResult:
    """Формирует Исход C с детализацией причин отсева."""
    prices = [float(item["price_from_kzt"]) for item in city_category_pool if isinstance(item.get("price_from_kzt"), Real) and not isinstance(item.get("price_from_kzt"), bool) and item["price_from_kzt"] > 0]
    minimum = min(prices, default=None)
    reasons = []
    if _count(filter_pipeline_results, "availability"):
        reasons.append(f"{_count(filter_pipeline_results, 'availability')} заняты на {_date(user_query.get('date', user_query.get('event_date')))}")
    if _count(filter_pipeline_results, "budget"):
        reasons.append(f"у {_count(filter_pipeline_results, 'budget')} цена выше бюджета")
    optional = filter_pipeline_results.get("optional_rejection_reasons", {})
    labels = {"event_format": "не подходят по формату", "language": "не подходят по языку", "duration_hours": "не подходят по длительности"}
    for key, label in labels.items():
        if isinstance(optional, dict) and optional.get(key, 0):
            reasons.append(f"{optional[key]} {label}")
    message = f"В городе {user_query.get('city', 'указанном городе')} найдено {len(city_category_pool)} подрядчиков категории {user_query.get('category', 'указанной категории')}, но подходящих вариантов не осталось."
    if reasons:
        message += " Причины: " + "; ".join(reasons) + "."
    if minimum is not None and user_query.get("budget") is not None:
        message += f" Минимальная цена — {minimum:,.0f} ₸, бюджет — {user_query['budget']:,.0f} ₸.".replace(",", " ")
    return SearchResult(SearchOutcome.NO_MATCHES_AFTER_FILTERS, [], message, {"total_in_city_category": len(city_category_pool), "min_price_in_category": minimum, "rejection_stats": filter_pipeline_results})


def explain_partial_results(found_count: int, total_in_city: int, rejection_stats: dict) -> str:
    """Объясняет выдачу из одного-двух результатов."""
    if found_count not in (1, 2):
        return "" if found_count >= 3 else "Количество найденных профилей должно быть неотрицательным"
    word = "подрядчик" if found_count == 1 else "подрядчика"
    rejected = total_in_city - found_count
    busy, budget = _count(rejection_stats, "availability"), _count(rejection_stats, "budget")
    reasons = []
    if busy: reasons.append(f"{busy} заняты на выбранную дату")
    if budget: reasons.append(f"{budget} превышают бюджет")
    suffix = "; ".join(reasons) if reasons else ("это все доступные профили" if rejected == 0 else f"{rejected} не прошли фильтрацию")
    return f"Найдено только {found_count} {word} (из {total_in_city}), так как {suffix}."


def process_partial_matches(filtered_candidates: list[dict], city_category_pool: list[dict], rejection_stats: dict, user_query: dict) -> dict:
    """Возвращает карточки и пояснение для неполной выдачи."""
    message = explain_partial_results(len(filtered_candidates), len(city_category_pool), rejection_stats)
    return {"outcome": "MATCH_FOUND", "cards": list(filtered_candidates), "message": message, "partial_explanation_message": message, "details": {"has_partial_explanation": True, "rejection_stats": rejection_stats}}


class ExplanationPromptBuilder:
    """Создаёт компактные system/user prompts для LLM."""
    SYSTEM_PROMPT = """Верни только валидный JSON-массив объектов {\"id\": ..., \"explanation\": ...}. Для каждого подрядчика дай ровно 1–2 предложения на русском языке. Используй только факты из description и точные совпадения с запросом: город, категория, цена, бюджет, формат, язык и длительность. Не выдумывай опыт, услуги, отзывы или гарантии. Запрещены штампы: «отличный выбор», «прекрасно подойдет», «профессионал своего дела», «идеальный вариант» и аналоги. Объяснения должны быть конкретными и различимыми для каждого профиля."""
    FIELDS = ("id", "anon_name", "categories", "city", "price_from_kzt", "event_formats", "languages", "max_hours", "description")

    def build_prompt(self, user_query: dict, selected_contractors: list[dict]) -> dict[str, str]:
        """Строит запрос для LLM из 1–3 профилей."""
        if not 1 <= len(selected_contractors) <= 3:
            raise ValueError("Нужно передать от 1 до 3 подрядчиков")
        profiles = [{field: profile.get(field) for field in self.FIELDS if field in profile} for profile in selected_contractors]
        compact = lambda value: json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), default=str)
        return {"system_prompt": self.SYSTEM_PROMPT, "user_prompt": f"ЗАПРОС: {compact(user_query)}\nПОДРЯДЧИКИ: {compact(profiles)}"}


_BANNED = ("отличный выбор", "замечательный вариант", "прекрасно подойдет", "прекрасно подойдёт", "профессионал своего дела", "идеальный вариант")


def _fallback(profile: dict) -> str:
    categories = ", ".join(str(x) for x in _categories(profile)) or "не указана"
    city = str(profile.get("city", "не указан")).strip()
    price = profile.get("price_from_kzt", "не указана")
    languages = ", ".join(str(x) for x in profile.get("languages", []) or [])
    return f"Категория: {categories}; город: {city}; цена от {price} ₸" + (f"; языки: {languages}." if languages else ".")


def validate_and_sanitize_explanation(explanation_text: str, contractor: dict) -> str:
    """Очищает explanation и заменяет невалидный текст на фактологический fallback."""
    text = explanation_text.strip().replace('"', "").replace("'", "") if isinstance(explanation_text, str) else ""
    text = " ".join(text.replace("\n", " ").split())
    count = len([part for part in re.split(r"[.!?]+", text) if part.strip()])
    return text if text and 1 <= count <= 2 and not any(item in text.casefold() for item in _BANNED) else _fallback(contractor)


def build_final_response_cards(selected_contractors: list[dict], raw_explanations: dict[Any, str], outcome: str, partial_message: str | None = None) -> dict:
    """Собирает строгое JSON-представление карточек."""
    allowed = {item.value for item in SearchOutcome}
    if outcome not in allowed:
        raise ValueError(f"Неизвестный outcome: {outcome}")
    cards = []
    for profile in selected_contractors[:3]:
        identifier = profile.get("id")
        explanation = raw_explanations.get(identifier, raw_explanations.get(str(identifier), ""))
        categories = _categories(profile)
        cards.append({"id": identifier, "anon_name": str(profile.get("anon_name", "")), "category": ", ".join(map(str, categories)), "city": str(profile.get("city", "")), "price_from_kzt": profile.get("price_from_kzt", 0), "explanation": validate_and_sanitize_explanation(explanation, profile), "is_synthetic": bool(profile.get("synthetic", False))})
    return {"outcome": outcome, "cards": cards, "message": partial_message or "", "partial_explanation": partial_message}
