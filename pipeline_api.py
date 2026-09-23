"""JSON-мост между Go API и Python-пайплайном: stdin -> stdout."""

from __future__ import annotations

import argparse
import contextlib
import hashlib
import json
import logging
import sys

from main_pipeline import run_pipeline
from src.data_loader import load_dataset


def search(dataset: list[dict], query: dict) -> dict:
    """Сохраняет формат карточек и метаданных, ожидаемый Soda_UI."""
    result = run_pipeline(dataset, query)
    details = result.get("details", {})
    stats = details.get("filter_stats", details.get("rejection_stats", {}))
    rejected = stats.get("rejected_by_step", {})
    optional = stats.get("optional_rejection_reasons", {})
    profiles = {profile["id"]: profile for profile in dataset}
    cards = [
        {**profiles[card["id"]], "matched_category": query["category"],
         "explanation": card["explanation"],
         "data_origin": profiles[card["id"]].get("data_origin", "organizer")}
        for card in result["cards"]
    ]
    total = stats.get("after_optional_criteria", 0)
    candidates = stats.get("after_city_category", 0)
    ranking = details.get("ranking", "budget")
    notice = ""
    if ranking == "hashing":
        notice = "Модель недоступна: пожелания сопоставляются по словам. Это не гарантирует наличие услуги."
    elif ranking == "semantic":
        notice = "Порядок учитывает близость описаний к пожеланиям; она не гарантирует наличие услуги."
    version = hashlib.sha256(json.dumps(dataset, ensure_ascii=False, sort_keys=True).encode("utf-8")).hexdigest()
    return {
        "status": {"MATCH_FOUND": "matches_found", "NO_CATEGORY_IN_CITY": "category_unavailable",
                   "NO_MATCHES_AFTER_FILTERS": "no_matches"}[result["outcome"]],
        "cards": cards, "total": total, "candidate_count": candidates,
        "excluded": {"booked": rejected.get("availability", 0), "budget": rejected.get("budget", 0),
                     "format": optional.get("event_format", 0), "language": optional.get("language", 0),
                     "duration": optional.get("duration_hours", 0)},
        "message": result.get("message") or f"Подходят {total} из {candidates}; показываем {len(cards)}.",
        "ranking": ranking, "notice": notice, "data_version": f"python-v1:{version}",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", default="data/hackathon-dataset-anonymized.jsonl")
    args = parser.parse_args()
    try:
        query = json.load(sys.stdin)
        if not isinstance(query, dict):
            raise ValueError("Ожидался JSON-объект запроса")
        # Library diagnostics must not corrupt the single JSON response on stdout.
        with contextlib.redirect_stdout(sys.stderr):
            if "query" in query:
                # The API sends a fresh database snapshot; JSONL is only for CLI use.
                dataset, query = query.get("dataset"), query["query"]
                if not isinstance(dataset, list) or not isinstance(query, dict):
                    raise ValueError("Ожидались список профилей и объект запроса")
            else:
                dataset = load_dataset(args.dataset)
            response = search(dataset, query)
        json.dump(response, sys.stdout, ensure_ascii=False, allow_nan=False)
        sys.stdout.write("\n")
        return 0
    except Exception:
        logging.exception("Не удалось выполнить поиск")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
