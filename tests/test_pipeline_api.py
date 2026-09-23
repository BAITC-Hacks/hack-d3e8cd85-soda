"""Контракт Python-ответа для веб-интерфейса и дробная длительность."""

import io
import json
import unittest
from unittest.mock import patch

from pipeline_api import main, search
from tests.test_e2e_pipeline import BASE_QUERY, DATASET


class PipelineAPITests(unittest.TestCase):
    def test_api_uses_supplied_snapshot_and_preserves_origin(self):
        dataset = [{**DATASET[0], "id": "live-database-profile", "data_origin": "team"}]
        output = io.StringIO()
        payload = json.dumps({"query": BASE_QUERY, "dataset": dataset})
        with patch("sys.argv", ["pipeline_api.py", "--dataset", "missing.jsonl"]), \
                patch("sys.stdin", io.StringIO(payload)), patch("sys.stdout", output):
            self.assertEqual(main(), 0)
        card = json.loads(output.getvalue())["cards"][0]
        self.assertEqual(card["id"], "live-database-profile")
        self.assertEqual(card["data_origin"], "team")

    def test_full_cards_and_fractional_hours(self):
        result = search(DATASET, {**BASE_QUERY, "duration_hours": 4.5})
        self.assertEqual(result["status"], "matches_found")
        self.assertEqual(result["total"], 1)
        self.assertEqual(result["candidate_count"], 2)
        self.assertEqual(result["excluded"]["booked"], 1)
        card = result["cards"][0]
        self.assertEqual(card["description"], DATASET[0]["description"])
        self.assertEqual(card["categories"], DATASET[0]["categories"])
        self.assertEqual(card["matched_category"], BASE_QUERY["category"])
        self.assertFalse(card["synthetic"])
        self.assertEqual(result["ranking"], "budget")

    def test_empty_outcomes_have_arrays_and_counts(self):
        absent = search(DATASET, {**BASE_QUERY, "category": "Фотобудка"})
        self.assertEqual(absent["status"], "category_unavailable")
        self.assertEqual(absent["cards"], [])
        self.assertEqual(absent["candidate_count"], 0)
        expensive = search(DATASET, {**BASE_QUERY, "budget": 1})
        self.assertEqual(expensive["status"], "no_matches")
        self.assertEqual(expensive["cards"], [])
        self.assertEqual(expensive["candidate_count"], 2)
        self.assertEqual(expensive["excluded"]["budget"], 1)

    def test_fallback_ranking_is_explicit(self):
        profiles = [{**DATASET[0], "id": str(index)} for index in range(4)]
        with patch("src.ranker.SemanticRanker._load_model", return_value=None):
            result = search(profiles, {**BASE_QUERY, "wishes": "интерактивная программа"})
        self.assertEqual(result["ranking"], "hashing")
        self.assertTrue(result["notice"])
        self.assertEqual(len(result["cards"]), 3)


if __name__ == "__main__":
    unittest.main()
