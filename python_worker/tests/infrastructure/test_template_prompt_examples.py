"""Sanity check: the curated YAML examples embedded in _TEMPLATE_GEN_PROMPT
must parse as valid YAML. If a future edit breaks one of them we'd ship a
broken prompt and the LLM would imitate broken syntax, so guard it here."""

from __future__ import annotations

import re

import yaml

from infrastructure.llm_client import _TEMPLATE_GEN_PROMPT

# Match fenced YAML blocks: ``` ... ```
_FENCE_RE = re.compile(r"```\n(.*?)\n```", re.DOTALL)


def _extract_yaml_blocks(prompt: str) -> list[str]:
    return _FENCE_RE.findall(prompt)


def test_prompt_has_at_least_two_curated_examples():
    blocks = _extract_yaml_blocks(_TEMPLATE_GEN_PROMPT)
    # Example A (Invoice) and Example B (PO). The "Good vs bad" section uses
    # short snippets, not full YAML blocks, so the fenced-block count should
    # be at least 2.
    assert len(blocks) >= 2, f"expected ≥2 fenced YAML examples, got {len(blocks)}"


def test_each_curated_example_parses_as_yaml():
    blocks = _extract_yaml_blocks(_TEMPLATE_GEN_PROMPT)
    for i, block in enumerate(blocks):
        try:
            data = yaml.safe_load(block)
        except yaml.YAMLError as exc:
            raise AssertionError(f"example #{i} is not valid YAML: {exc}\n\n{block}")
        assert isinstance(data, dict), f"example #{i} did not parse to a dict"
        assert "issuer" in data, f"example #{i} missing 'issuer'"
        assert "keywords" in data, f"example #{i} missing 'keywords'"
        assert "fields" in data, f"example #{i} missing 'fields'"


def test_each_curated_example_uses_inline_si_flag_on_field_regexes():
    """Every field regex in every example must start with `(?si)` — the
    convention the prompt teaches the LLM to follow."""
    blocks = _extract_yaml_blocks(_TEMPLATE_GEN_PROMPT)
    for i, block in enumerate(blocks):
        data = yaml.safe_load(block)
        fields = data.get("fields", {})
        for field, value in fields.items():
            patterns = value if isinstance(value, list) else [value]
            for pat in patterns:
                assert isinstance(pat, str), f"example #{i} field {field}: non-string pattern"
                assert pat.startswith("(?si)"), (
                    f"example #{i} field {field}: regex missing inline (?si) flag: {pat!r}"
                )


def test_example_a_includes_iso_date_regex_variant():
    """Example A must include an ISO date regex so the LLM learns to handle
    YYYY-MM-DD format invoices (the root cause of the 2026-05-08 DLQ failures)."""
    blocks = _extract_yaml_blocks(_TEMPLATE_GEN_PROMPT)
    # Example A is the first fenced block (Invoice with HSN table)
    example_a = yaml.safe_load(blocks[0])
    date_patterns = example_a.get("fields", {}).get("date", [])
    if isinstance(date_patterns, str):
        date_patterns = [date_patterns]

    has_iso = any(r"\d{4}" in p for p in date_patterns)
    assert has_iso, (
        "Example A must include an ISO date regex variant containing \\d{4}: "
        f"found patterns: {date_patterns}"
    )


def test_example_a_options_include_iso_date_format():
    """Example A's date_formats must include %Y-%m-%d (ISO) so dateparser
    correctly interprets the matched ISO date string."""
    blocks = _extract_yaml_blocks(_TEMPLATE_GEN_PROMPT)
    example_a = yaml.safe_load(blocks[0])
    date_formats = example_a.get("options", {}).get("date_formats", [])
    assert "%Y-%m-%d" in date_formats, (
        "Example A options.date_formats must include '%Y-%m-%d'"
    )
