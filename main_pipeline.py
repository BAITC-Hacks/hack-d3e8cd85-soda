"""Единая точка входа для подбора подрядчиков."""

from __future__ import annotations

import datetime
import json
from typing import Any

from src.data_loader import load_dataset, normalize_dataset_dates, normalize_profile_fields, normalize_user_query, parse_date, process_synthetic_flags
from src.filters import filter_by_city_and_category, run_hard_filters
from src.generator import ExplanationPromptBuilder, analyze_no_matches_outcome, build_final_response_cards, check_category_in_city_outcome, process_partial_matches
from src.ranker import get_query_hash, rank_and_select_top_candidates, SemanticRanker


def _query_values(query: dict, *keys: str) -> Any:
    for key in keys:
        value = query.get(key)
        if isinstance(value, list):
            if value:
                return value[0]
        elif value not in (None, ""):
            return value
    return None


def run_pipeline(dataset: list[dict], user_query: dict, raw_explanations: Any = None, semantic_ranker: SemanticRanker | None = None) -> dict:
    """Выполняет полный путь от нормализации до финального JSON-ответа."""
    profiles = normalize_dataset_dates([process_synthetic_flags(normalize_profile_fields(profile)) for profile in dataset])
    query = normalize_user_query(user_query)
    city = str(query.get("city", ""))
    category = _query_values(query, "categories", "category") or ""
    event_format = _query_values(query, "event_formats", "event_format")
    language = _query_values(query, "languages", "language")
    date_value = query.get("event_date", query.get("date"))
    event_date = date_value if isinstance(date_value, datetime.date) else parse_date(str(date_value))
    if event_date is None:
        raise ValueError("Некорректная дата мероприятия")
    compatibility_query = {**query, "category": category, "event_format": event_format, "language": language, "date": date_value, "event_date": event_date}

    early = check_category_in_city_outcome(profiles, city, category)
    if early is not None:
        return early.as_dict()
    pool = filter_by_city_and_category(profiles, city, category)
    filtered, stats = run_hard_filters(profiles, city, category, event_date, query["budget"], event_format, language, query.get("duration_hours"))
    if not filtered:
        return analyze_no_matches_outcome(pool, compatibility_query, stats).as_dict()

    text_query = query.get("wishes", query.get("text", ""))
    ranking = "budget"
    if len(filtered) > 3 and isinstance(text_query, str) and text_query.strip():
        ranker = semantic_ranker or SemanticRanker()
        ranked = ranker.rank_candidates_by_semantic_similarity(filtered, text_query, 3)
        ranking = "semantic" if ranker.model is not None else "hashing"
    else:
        ranked = rank_and_select_top_candidates(filtered, compatibility_query, 3)
    partial_message = None
    if len(filtered) <= 2:
        partial_message = process_partial_matches(ranked, pool, stats, compatibility_query)["partial_explanation_message"]

    ExplanationPromptBuilder().build_prompt(compatibility_query, ranked)
    explanations = {}
    if isinstance(raw_explanations, str):
        try:
            raw_explanations = json.loads(raw_explanations)
        except json.JSONDecodeError:
            raw_explanations = []
    if isinstance(raw_explanations, list):
        explanations = {item["id"]: item["explanation"] for item in raw_explanations if isinstance(item, dict) and "id" in item and isinstance(item.get("explanation"), str)}
    elif isinstance(raw_explanations, dict):
        explanations = raw_explanations
    response = build_final_response_cards(ranked, explanations, "MATCH_FOUND", partial_message)
    response["details"] = {"query_hash": get_query_hash(compatibility_query), "filter_stats": stats, "ranking": ranking}
    return response


def run_pipeline_from_file(file_path: str, user_query: dict, raw_explanations: Any = None) -> dict:
    """Загружает каталог и запускает pipeline."""
    return run_pipeline(load_dataset(file_path), user_query, raw_explanations)


if __name__ == "__main__":
    path = "data/hackathon-dataset-anonymized.jsonl"
    print("Pipeline готов. Передайте user_query и вызовите run_pipeline_from_file().")
