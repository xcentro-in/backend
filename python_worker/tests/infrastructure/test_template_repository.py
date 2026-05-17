"""Tests for PostgresTemplateRepository.

Same mock-cursor pattern as test_database.py — we verify the SQL shape and
return-value mapping without a live Postgres.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch


def _build_repo():
    pool = MagicMock()
    conn = MagicMock()
    cursor = MagicMock()

    pool.getconn.return_value = conn
    pool.closed = False
    conn.closed = False
    conn.cursor.return_value.__enter__.return_value = cursor
    conn.cursor.return_value.__exit__.return_value = False

    with patch("infrastructure.template_repository.pg_pool.SimpleConnectionPool", return_value=pool):
        from infrastructure.template_repository import PostgresTemplateRepository

        repo = PostgresTemplateRepository(dsn="host=x")

    return repo, pool, conn, cursor


def test_list_all_returns_rows_as_list_of_dicts():
    repo, _pool, _conn, cursor = _build_repo()
    cursor.fetchall.return_value = [
        {"name": "a.yml", "content": "issuer: A\n", "issuer": "A"},
        {"name": "b.yml", "content": "issuer: B\n", "issuer": "B"},
    ]

    out = repo.list_all()

    assert out == [
        {"name": "a.yml", "content": "issuer: A\n", "issuer": "A"},
        {"name": "b.yml", "content": "issuer: B\n", "issuer": "B"},
    ]
    sql = cursor.execute.call_args[0][0]
    assert "SELECT name, content, issuer FROM xcentro.templates" in sql
    assert "ORDER BY name" in sql


def test_get_by_name_returns_content_when_found():
    repo, _pool, _conn, cursor = _build_repo()
    cursor.fetchone.return_value = ("issuer: Acme\nkeywords:\n  - TAX INVOICE\n",)

    content = repo.get_by_name("acme.yml")

    assert content.startswith("issuer: Acme")
    _sql, params = cursor.execute.call_args[0]
    assert params == ("acme.yml",)


def test_get_by_name_returns_none_when_missing():
    repo, _pool, _conn, cursor = _build_repo()
    cursor.fetchone.return_value = None

    assert repo.get_by_name("missing.yml") is None


def test_save_uses_upsert_sql_and_passes_params():
    repo, _pool, _conn, cursor = _build_repo()

    repo.save(name="x.yml", content="issuer: X\n", issuer="X")

    sql, params = cursor.execute.call_args[0]
    assert "INSERT INTO xcentro.templates" in sql
    assert "ON CONFLICT (name) DO UPDATE" in sql
    assert params == {
        "name": "x.yml",
        "content": "issuer: X\n",
        "issuer": "X",
        "replaced_by": "regen",
    }


def test_save_allows_null_issuer():
    repo, _pool, _conn, cursor = _build_repo()

    repo.save(name="y.yml", content="keywords: []\n")

    _sql, params = cursor.execute.call_args[0]
    assert params == {
        "name": "y.yml",
        "content": "keywords: []\n",
        "issuer": None,
        "replaced_by": "regen",
    }


def test_save_writes_history_snapshot_via_cte():
    """The upsert SQL must capture the prior row into templates_history before
    overwriting it; we assert the SQL shape since we can't run a real DB."""
    repo, _pool, _conn, cursor = _build_repo()

    repo.save(name="z.yml", content="issuer: Z\n", issuer="Z", replaced_by="hitl_regen")

    sql, params = cursor.execute.call_args[0]
    assert "WITH prior AS" in sql
    assert "FROM xcentro.templates" in sql and "WHERE name = %(name)s" in sql
    assert "INSERT INTO xcentro.templates_history" in sql
    assert "INSERT INTO xcentro.templates" in sql
    assert "ON CONFLICT (name) DO UPDATE" in sql
    assert params["replaced_by"] == "hitl_regen"


def test_close_is_idempotent():
    repo, pool, _conn, _cursor = _build_repo()

    repo.close()
    pool.closeall.assert_called_once()

    pool.closed = True
    repo.close()
    pool.closeall.assert_called_once()
