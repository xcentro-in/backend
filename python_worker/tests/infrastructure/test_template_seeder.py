"""Tests for seed_from_filesystem.

The seeder walks a directory of .yml templates and inserts any that are not
already in the repository. It must be idempotent (DB copies always win) and
resilient to malformed YAML or unreadable files, since an unrecoverable seed
error would block worker startup.
"""

from __future__ import annotations

import builtins
import os

import pytest

from infrastructure.template_seeder import seed_from_filesystem


class _StubRepo:
    """Minimal TemplateRepository stub: dict storage + call recording."""

    def __init__(self, preseeded: dict[str, str] | None = None) -> None:
        self.store: dict[str, dict] = {}
        self.save_calls: list[tuple[str, str, str | None]] = []
        if preseeded:
            for name, content in preseeded.items():
                self.store[name] = {"name": name, "content": content, "issuer": None}

    def list_all(self) -> list[dict]:
        return list(self.store.values())

    def save(self, name: str, content: str, issuer: str | None = None) -> None:
        self.store[name] = {"name": name, "content": content, "issuer": issuer}
        self.save_calls.append((name, content, issuer))

    def get_by_name(self, name: str) -> str | None:
        entry = self.store.get(name)
        return entry["content"] if entry else None


def _write(path, content: str) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(content)


def test_seeds_new_files(tmp_path):
    _write(tmp_path / "a.yml", "issuer: Acme\nkeywords:\n  - TAX INVOICE\n")
    _write(tmp_path / "b.yml", "issuer: Beta Corp\nkeywords: [PO]\n")
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path), repo)

    assert len(repo.save_calls) == 2
    names = {c[0] for c in repo.save_calls}
    assert names == {"a.yml", "b.yml"}
    issuers = {c[0]: c[2] for c in repo.save_calls}
    assert issuers["a.yml"] == "Acme"
    assert issuers["b.yml"] == "Beta Corp"


def test_skips_already_present_templates(tmp_path):
    _write(tmp_path / "existing.yml", "issuer: Old\n")
    _write(tmp_path / "fresh.yml", "issuer: New\n")
    repo = _StubRepo(preseeded={"existing.yml": "DB-version-wins"})

    seed_from_filesystem(str(tmp_path), repo)

    assert len(repo.save_calls) == 1
    assert repo.save_calls[0][0] == "fresh.yml"
    # DB copy is untouched — this is the whole point of the seeder.
    assert repo.get_by_name("existing.yml") == "DB-version-wins"


def test_missing_dir_returns_silently(tmp_path):
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path / "does-not-exist"), repo)

    assert repo.save_calls == []


def test_empty_dir_returns_silently(tmp_path):
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path), repo)

    assert repo.save_calls == []


def test_malformed_yaml_still_seeds_with_null_issuer(tmp_path):
    _write(tmp_path / "bad.yml", "::: not yaml :::\nkey: [unterminated")
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path), repo)

    assert len(repo.save_calls) == 1
    name, _content, issuer = repo.save_calls[0]
    assert name == "bad.yml"
    assert issuer is None


def test_unreadable_file_is_logged_not_fatal(tmp_path, monkeypatch):
    _write(tmp_path / "good.yml", "issuer: OK\n")
    _write(tmp_path / "bad.yml", "issuer: NopeNeverRead\n")
    repo = _StubRepo()

    real_open = builtins.open

    def flaky_open(path, *args, **kwargs):
        if str(path).endswith("bad.yml"):
            raise OSError("permission denied")
        return real_open(path, *args, **kwargs)

    monkeypatch.setattr(builtins, "open", flaky_open)

    seed_from_filesystem(str(tmp_path), repo)

    # Good file still seeded; bad one logged + skipped, no exception raised.
    assert len(repo.save_calls) == 1
    assert repo.save_calls[0][0] == "good.yml"


def test_only_yml_extension_considered(tmp_path):
    _write(tmp_path / "real.yml", "issuer: R\n")
    _write(tmp_path / "wrong.yaml", "issuer: X\n")
    _write(tmp_path / "ignored.txt", "issuer: X\n")
    _write(tmp_path / "backup.yml.bak", "issuer: X\n")
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path), repo)

    assert len(repo.save_calls) == 1
    assert repo.save_calls[0][0] == "real.yml"


def test_issuer_defaults_to_none_when_yaml_has_no_top_level_dict(tmp_path):
    # YAML parses to a list, not a dict — issuer field can't be pulled.
    _write(tmp_path / "listy.yml", "- one\n- two\n")
    repo = _StubRepo()

    seed_from_filesystem(str(tmp_path), repo)

    assert len(repo.save_calls) == 1
    _name, _content, issuer = repo.save_calls[0]
    assert issuer is None
