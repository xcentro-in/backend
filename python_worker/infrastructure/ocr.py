"""Shared OCR helpers for the extraction pipeline."""

from __future__ import annotations

import dataclasses
import logging
import re
from typing import Any

from invoice2data.input import pdftotext

logger = logging.getLogger(__name__)

_OCR_HINT_KEYWORDS = (
    "invoice",
    "tax invoice",
    "purchase order",
    "po",
    "gst",
    "gstin",
    "bill to",
    "ship to",
    "total",
    "amount",
    "qty",
    "description",
    "cgst",
    "sgst",
    "igst",
    "subtotal",
)
_LOW_QUALITY_SCORE = 2.2
_TESSERACT_WIN_MARGIN = 0.25


@dataclasses.dataclass(frozen=True)
class OCRMetrics:
    """Basic quality signals used to compare OCR candidates."""

    length: int
    token_count: int
    alnum_ratio: float
    keyword_hits: int
    replacement_chars: int


@dataclasses.dataclass(frozen=True)
class OCRResult:
    """Selected OCR output plus debug metadata."""

    text: str
    engine: str
    input_module: Any | None
    score: float
    reason: str
    pdftotext_score: float
    tesseract_score: float | None = None


@dataclasses.dataclass(frozen=True)
class _OCRCandidate:
    text: str
    engine: str
    input_module: Any | None
    score: float
    metrics: OCRMetrics
    error: str | None = None


def extract_best_ocr(file_path: str) -> OCRResult:
    """Extract OCR text and choose the best available engine output."""
    pdf_candidate = _run_engine(file_path, "pdftotext", pdftotext)

    if not _should_try_tesseract(pdf_candidate):
        return OCRResult(
            text=pdf_candidate.text,
            engine=pdf_candidate.engine,
            input_module=pdf_candidate.input_module,
            score=pdf_candidate.score,
            reason=(
                "Using pdftotext because the extracted text quality looks acceptable "
                f"({_describe_metrics(pdf_candidate.metrics)})."
            ),
            pdftotext_score=pdf_candidate.score,
        )

    tesseract_candidate = _run_engine(file_path, "tesseract", _load_tesseract_module())
    selected = _pick_best_candidate(pdf_candidate, tesseract_candidate)

    if selected.engine == "tesseract":
        reason = (
            "Using tesseract because pdftotext text looked low quality "
            f"({_describe_metrics(pdf_candidate.metrics)}) and tesseract scored better "
            f"({tesseract_candidate.score:.2f} vs {pdf_candidate.score:.2f})."
        )
    elif tesseract_candidate.error:
        reason = (
            "Keeping pdftotext because tesseract was unavailable or failed after "
            f"pdftotext produced low-quality text ({tesseract_candidate.error})."
        )
    else:
        reason = (
            "Keeping pdftotext after comparing against tesseract because it remained "
            f"the stronger OCR output ({pdf_candidate.score:.2f} vs {tesseract_candidate.score:.2f})."
        )

    return OCRResult(
        text=selected.text,
        engine=selected.engine,
        input_module=selected.input_module,
        score=selected.score,
        reason=reason,
        pdftotext_score=pdf_candidate.score,
        tesseract_score=tesseract_candidate.score,
    )


def _pick_best_candidate(pdf_candidate: _OCRCandidate, tesseract_candidate: _OCRCandidate) -> _OCRCandidate:
    if tesseract_candidate.text and not pdf_candidate.text:
        return tesseract_candidate
    if not tesseract_candidate.text:
        return pdf_candidate
    if tesseract_candidate.score >= pdf_candidate.score + _TESSERACT_WIN_MARGIN:
        return tesseract_candidate
    return pdf_candidate


def _should_try_tesseract(candidate: _OCRCandidate) -> bool:
    return not candidate.text or candidate.score < _LOW_QUALITY_SCORE


def _run_engine(file_path: str, engine_name: str, input_module: Any | None) -> _OCRCandidate:
    if input_module is None:
        return _OCRCandidate(
            text="",
            engine=engine_name,
            input_module=None,
            score=0.0,
            metrics=_measure_text_quality(""),
            error=f"{engine_name} module unavailable",
        )

    try:
        raw = input_module.to_text(file_path)
        text = _clean_text(raw)
        metrics = _measure_text_quality(text)
        return _OCRCandidate(
            text=text,
            engine=engine_name,
            input_module=input_module,
            score=_score_text_quality(metrics),
            metrics=metrics,
        )
    except (OSError, RuntimeError, ImportError) as exc:
        logger.warning("%s failed for %s: %s", engine_name, file_path, exc)
        return _OCRCandidate(
            text="",
            engine=engine_name,
            input_module=input_module,
            score=0.0,
            metrics=_measure_text_quality(""),
            error=str(exc),
        )


def _load_tesseract_module() -> Any | None:
    try:
        from invoice2data.input import tesseract
    except (ImportError, OSError) as exc:
        logger.warning("Unable to load tesseract OCR module: %s", exc)
        return None
    return tesseract


def _clean_text(raw: Any) -> str:
    if raw is None:
        return ""
    if isinstance(raw, bytes):
        raw = raw.decode("utf-8", errors="replace")
    return str(raw).replace("\x00", "").strip()


def _measure_text_quality(text: str) -> OCRMetrics:
    stripped = text.strip()
    non_space_chars = [ch for ch in stripped if not ch.isspace()]
    non_space_len = len(non_space_chars)
    alnum_count = sum(ch.isalnum() for ch in non_space_chars)
    token_count = len(re.findall(r"[A-Za-z0-9]{2,}", stripped))
    lower = stripped.lower()
    keyword_hits = sum(1 for kw in _OCR_HINT_KEYWORDS if kw in lower)
    replacement_chars = stripped.count("\ufffd")
    alnum_ratio = (alnum_count / non_space_len) if non_space_len else 0.0
    return OCRMetrics(
        length=len(stripped),
        token_count=token_count,
        alnum_ratio=alnum_ratio,
        keyword_hits=keyword_hits,
        replacement_chars=replacement_chars,
    )


def _score_text_quality(metrics: OCRMetrics) -> float:
    if metrics.length == 0:
        return 0.0

    score = 0.0
    score += min(metrics.length / 400.0, 1.5)
    score += min(metrics.token_count / 40.0, 1.5)
    score += metrics.alnum_ratio * 2.0
    score += min(metrics.keyword_hits * 0.25, 1.5)
    score -= min(metrics.replacement_chars * 0.3, 1.5)

    if metrics.length < 80:
        score -= 0.75
    if metrics.token_count < 12:
        score -= 0.75
    if metrics.alnum_ratio < 0.45:
        score -= 0.75

    return max(score, 0.0)


def _describe_metrics(metrics: OCRMetrics) -> str:
    return (
        f"chars={metrics.length}, tokens={metrics.token_count}, "
        f"alnum_ratio={metrics.alnum_ratio:.2f}, keyword_hits={metrics.keyword_hits}"
    )
