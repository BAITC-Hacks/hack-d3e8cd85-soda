"""Доступный E2E smoke-test Python pipeline и его JSON-контракта."""

from __future__ import annotations

import time

from main_pipeline import run_pipeline


DATASET = [
    {
        "id": "A-1",
        "anon_name": "Ведущий Алматы",
        "categories": ["Ведущий"],
        "city": "Алматы",
        "price_from_kzt": 100_000,
        "event_formats": ["свадьба"],
        "languages": ["русский"],
        "max_hours": 8,
        "busy_dates": [],
        "description": "Интерактивная программа с конкурсами.",
        "synthetic": False,
        "city_imputed": False,
        "price_imputed": False,
    },
    {
        "id": "C-1",
        "anon_name": "Занятый ведущий",
        "categories": ["Ведущий"],
        "city": "Алматы",
        "price_from_kzt": 100_000,
        "event_formats": ["свадьба"],
        "languages": ["русский"],
        "max_hours": 8,
        "busy_dates": ["2026-11-14"],
        "description": "Ведущий с музыкальными конкурсами.",
        "synthetic": True,
        "city_imputed": False,
        "price_imputed": False,
    },
]

BASE_QUERY = {
    "city": "Алматы",
    "category": "Ведущий",
    "event_date": "2026-11-14",
    "budget": 200_000,
    "event_format": "свадьба",
    "language": "русский",
    "duration_hours": 4,
}


def test_python_e2e_outcomes() -> None:
    """Проверяет A, B, C и обязательные поля карточки."""
    started = time.perf_counter()
    success = run_pipeline(DATASET, BASE_QUERY)
    outcome_b = run_pipeline(DATASET, {**BASE_QUERY, "category": "Фотобудка"})
    outcome_c = run_pipeline(DATASET, {**BASE_QUERY, "budget": 50_000})
    elapsed = time.perf_counter() - started

    assert success["outcome"] == "MATCH_FOUND"
    assert len(success["cards"]) == 1
    assert {"id", "anon_name", "category", "city", "price_from_kzt", "explanation", "is_synthetic"} == set(success["cards"][0])
    assert success["cards"][0]["is_synthetic"] is False
    assert outcome_b["outcome"] == "NO_CATEGORY_IN_CITY"
    assert outcome_b["cards"] == []
    assert outcome_c["outcome"] == "NO_MATCHES_AFTER_FILTERS"
    assert outcome_c["cards"] == []
    assert elapsed < 10.0


if __name__ == "__main__":
    test_python_e2e_outcomes()
    print("Python E2E pipeline test passed")
