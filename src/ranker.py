"""Детерминированное и семантическое ранжирование."""

from __future__ import annotations

import hashlib
import importlib
import json
import logging
import math
import re
from numbers import Real
from typing import Any, Protocol, Sequence

from .data_loader import clean_text

logger = logging.getLogger(__name__)


def _json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), default=str)


def get_query_hash(user_query: dict) -> str:
    """Возвращает стабильный SHA-256 хеш запроса."""
    return hashlib.sha256(_json(user_query).encode("utf-8")).hexdigest()


def _values(value: Any) -> set[str]:
    raw = [value] if isinstance(value, str) else value if isinstance(value, (list, tuple, set)) else []
    return {clean_text(item) for item in raw if clean_text(item)}


def _price(profile: dict) -> float | None:
    value = profile.get("price_from_kzt")
    if isinstance(value, bool) or not isinstance(value, Real):
        return None
    value = float(value)
    return value if math.isfinite(value) else None


def rank_and_select_top_candidates(candidates: list[dict], user_query: dict, limit: int = 3) -> list[dict]:
    """Сортирует по близости к бюджету, совпадениям и стабильным ключам."""
    if isinstance(limit, bool) or not isinstance(limit, int) or limit < 0:
        raise ValueError("limit должен быть неотрицательным int")
    budget = user_query.get("budget")
    budget = float(budget) if isinstance(budget, Real) and not isinstance(budget, bool) and math.isfinite(float(budget)) else None
    formats = _values(user_query.get("event_format", user_query.get("event_formats")))
    languages = _values(user_query.get("language", user_query.get("languages")))

    def key(item: tuple[int, dict]) -> tuple[Any, ...]:
        index, profile = item
        price = _price(profile)
        budget_key = budget - price if budget is not None and price is not None else float("inf")
        matches = bool(formats & _values(profile.get("event_formats"))) + bool(languages & _values(profile.get("languages")))
        completeness = sum(bool(profile.get(field)) for field in ("description", "city", "categories", "event_formats", "languages"))
        return (budget_key, -matches, -len(clean_text(profile.get("description"))), -completeness, str(profile.get("id", "")), clean_text(profile.get("anon_name")), index)

    return [profile for _, profile in sorted(enumerate(candidates), key=key)[:limit]]


class EmbeddingModel(Protocol):
    def encode(self, texts: Sequence[str], **kwargs: Any) -> Any: ...


class SemanticRanker:
    """Ранжирует описания по cosine similarity и кэширует эмбеддинги."""

    def __init__(self, model: EmbeddingModel | None = None, model_name: str = "intfloat/multilingual-e5-small", embedding_dimension: int = 256) -> None:
        self.cache: dict[str, tuple[float, ...]] = {}
        self.embedding_dimension = embedding_dimension
        self.model = model if model is not None else self._load_model(model_name)

    @staticmethod
    def _load_model(model_name: str) -> EmbeddingModel | None:
        try:
            module = importlib.import_module("sentence_transformers")
            return getattr(module, "SentenceTransformer")(model_name)
        except ImportError:
            logger.warning("sentence-transformers не установлен; используется hashing fallback")
            return None

    def _fallback(self, text: str) -> tuple[float, ...]:
        vector = [0.0] * self.embedding_dimension
        for token in re.findall(r"[\w]+", text.lower(), re.UNICODE):
            digest = hashlib.sha256(token.encode()).digest()
            vector[int.from_bytes(digest[:4], "big") % self.embedding_dimension] += 1.0 if digest[4] % 2 else -1.0
        return tuple(vector)

    def embed_text(self, text: str) -> tuple[float, ...]:
        if self.model is None:
            return self._fallback(text)
        value = self.model.encode([text], normalize_embeddings=False)
        if hasattr(value, "tolist"):
            value = value.tolist()
        if value and isinstance(value[0], (list, tuple)):
            value = value[0]
        return tuple(float(item) for item in value)

    def cosine_similarity(self, first: Sequence[float], second: Sequence[float]) -> float:
        if not first or not second or len(first) != len(second):
            return 0.0
        norm_first = math.sqrt(sum(value * value for value in first))
        norm_second = math.sqrt(sum(value * value for value in second))
        return sum(a * b for a, b in zip(first, second)) / (norm_first * norm_second) if norm_first and norm_second else 0.0

    def rank_candidates_by_semantic_similarity(self, candidates: list[dict], user_text_query: str, top_k: int = 3) -> list[dict]:
        """Возвращает top-k с ``semantic_score`` и стабильными tie-breaker’ами."""
        if top_k < 0:
            raise ValueError("top_k должен быть неотрицательным")
        if not isinstance(user_text_query, str) or not user_text_query.strip():
            return [{**item, "semantic_score": 0.0} for item in rank_and_select_top_candidates(candidates, {}, top_k)]
        query_vector = self.embed_text(user_text_query.strip())
        ranked = []
        for index, candidate in enumerate(candidates):
            description = candidate.get("description", "")
            if not isinstance(description, str) or len(description.strip()) < 3:
                score = 0.0
            else:
                vector = self.cache.setdefault(description.strip(), self.embed_text(description.strip()))
                score = self.cosine_similarity(query_vector, vector)
            ranked.append((score, candidate, index))
        ranked.sort(key=lambda item: (-item[0], str(item[1].get("id", "")), clean_text(item[1].get("anon_name")), item[2]))
        return [{**candidate, "semantic_score": score} for score, candidate, _ in ranked[:top_k]]


def rank_candidates_by_semantic_similarity(candidates: list[dict], user_text_query: str, top_k: int = 3) -> list[dict]:
    """Обёртка для одноразового семантического ранжирования."""
    return SemanticRanker().rank_candidates_by_semantic_similarity(candidates, user_text_query, top_k)
