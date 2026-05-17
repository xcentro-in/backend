# Improvement Plan 04 — Template Version History

**Priority:** Medium  
**Effort:** Medium (1–2 days)  
**Impact:** Safety, auditability, rollback capability  

---

## Problem

When a template is regenerated (because a human validated a document and the HITL loop triggered), the new YAML silently overwrites the previous one in `xcentro.templates`:

```sql
INSERT INTO xcentro.templates (name, content, issuer)
VALUES (%s, %s, %s)
ON CONFLICT (name) DO UPDATE
    SET content = EXCLUDED.content,
        issuer  = EXCLUDED.issuer,
        updated_at = NOW()
```

This means:
- A bad regeneration (LLM produces a worse template) permanently destroys the working one.
- There is no way to see what the template looked like before.
- There is no way to audit when and why a template changed.
- If the round-trip verifier passes but the template is subtly wrong (e.g., it works on the source document but fails on edge cases), there is no rollback mechanism.

---

## Proposed Solution

Add a `xcentro.template_versions` table to store the full history of every template change.

### Schema

```sql
CREATE TABLE xcentro.template_versions (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,             -- template filename (FK to templates.name)
    content     TEXT NOT NULL,             -- YAML content at this version
    issuer      TEXT,
    created_by  UUID,                      -- document_id that triggered this version
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_current  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_template_versions_name ON xcentro.template_versions(name);
```

### Modified Save Flow

```python
def save(self, name: str, content: str, issuer: str | None = None,
         created_by: str | None = None) -> None:
    with self._conn() as conn:
        with conn.cursor() as cur:
            # Mark all previous versions of this template as not current
            cur.execute(
                "UPDATE xcentro.template_versions SET is_current = FALSE WHERE name = %s",
                (name,)
            )
            # Insert new version
            cur.execute(
                """
                INSERT INTO xcentro.template_versions (name, content, issuer, created_by)
                VALUES (%s, %s, %s, %s)
                """,
                (name, content, issuer, created_by)
            )
            # Upsert the current templates table (unchanged behaviour)
            cur.execute(
                """
                INSERT INTO xcentro.templates (name, content, issuer)
                VALUES (%s, %s, %s)
                ON CONFLICT (name) DO UPDATE
                    SET content = EXCLUDED.content,
                        issuer  = EXCLUDED.issuer,
                        updated_at = NOW()
                """,
                (name, content, issuer)
            )
```

### Rollback API (Go side)

The Go API should expose an ops endpoint:
```
GET  /ops/templates                         → list all templates with version count
GET  /ops/templates/:name/versions          → list version history for a template
POST /ops/templates/:name/rollback/:version_id → restore a prior version
```

The rollback operation is simply:
```sql
UPDATE xcentro.templates
SET content = (SELECT content FROM xcentro.template_versions WHERE id = :version_id),
    updated_at = NOW()
WHERE name = :name;
```

### Files to Modify

- `infrastructure/template_repository.py` — Update `save()` to write version history; add `list_versions(name)` and `restore_version(name, version_id)` methods
- `domain/ports.py` — Add `list_versions()` and `restore_version()` to `TemplateRepository` ABC
- Database migration — Add `xcentro.template_versions` table

### Acceptance Criteria

- Every template save produces a new row in `template_versions`.
- `GET /ops/templates/:name/versions` returns a list of version timestamps + YAML previews.
- An ops user can restore a prior template version via one API call.
- The restored version takes effect immediately (no worker restart needed — the signature-based cache invalidates on next extract call).

---

## Why This Matters

Templates are the accumulated intelligence of the system. Each one represents a validated human-approved extraction for a real vendor. Overwriting them without history is equivalent to force-pushing over a git branch without backup. Version history is the audit trail and the safety net.
