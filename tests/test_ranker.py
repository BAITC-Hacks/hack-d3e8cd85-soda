"""Регрессионные проверки отказа модели и кэширования эмбеддингов."""

from __future__ import annotations

import unittest
from collections import Counter
from types import SimpleNamespace
from unittest.mock import Mock, patch

from main_pipeline import run_pipeline
from src.ranker import SemanticRanker
from tests.test_e2e_pipeline import BASE_QUERY, DATASET


class SemanticRankerTests(unittest.TestCase):
    def assert_fallback_search(self) -> None:
        profiles = [{**DATASET[0], "id": str(index)} for index in range(4)]
        query = {**BASE_QUERY, "wishes": "Интерактивная программа"}
        with self.assertLogs("src.ranker", level="WARNING"):
            ranker = SemanticRanker()
        self.assertIsNone(ranker.model)
        first = run_pipeline(profiles, query, semantic_ranker=ranker)
        second = run_pipeline(profiles, query, semantic_ranker=ranker)
        self.assertEqual(first["outcome"], "MATCH_FOUND")
        self.assertEqual(len(first["cards"]), 3)
        self.assertEqual(first, second)

    def test_missing_dependency_uses_fallback(self) -> None:
        with patch("src.ranker.importlib.import_module", side_effect=ImportError("not installed")):
            self.assert_fallback_search()

    def test_model_load_failure_uses_fallback(self) -> None:
        for error in (OSError("offline"), RuntimeError("invalid weights"), ValueError("invalid config")):
            with self.subTest(error=type(error).__name__):
                module = SimpleNamespace(SentenceTransformer=Mock(side_effect=error))
                with patch("src.ranker.importlib.import_module", return_value=module):
                    self.assert_fallback_search()

    def test_description_vectors_are_reused_across_candidates_and_queries(self) -> None:
        vectors = {
            "calm": [1.0, 0.0],
            "lively": [0.0, 1.0],
            "calm program": [1.0, 0.0],
            "lively program": [0.0, 1.0],
        }
        model = Mock()
        model.encode.side_effect = lambda texts, **kwargs: [vectors[text] for text in texts]
        ranker = SemanticRanker(model=model)
        profiles = [
            {"id": "A", "description": "calm program"},
            {"id": "B", "description": "  calm program  "},
            {"id": "C", "description": "lively program"},
        ]

        calm = ranker.rank_candidates_by_semantic_similarity(profiles, "calm")
        lively = ranker.rank_candidates_by_semantic_similarity(profiles, "lively")

        self.assertEqual([item["id"] for item in calm], ["A", "B", "C"])
        self.assertEqual([item["id"] for item in lively], ["C", "A", "B"])
        encoded = Counter(text for call in model.encode.call_args_list for text in call.args[0])
        self.assertEqual(encoded["calm program"], 1)
        self.assertEqual(encoded["lively program"], 1)


if __name__ == "__main__":
    unittest.main()
