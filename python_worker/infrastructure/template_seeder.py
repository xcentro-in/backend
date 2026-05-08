"""Seeds the database with invoice2data templates from the filesystem.

Runs once at worker startup. Idempotent — uses ON CONFLICT DO NOTHING so
existing templates are never overwritten by the seed, preserving any LLM
improvements that were made after initial seeding.
"""

from __future__ import annotations

import logging
import os

import yaml

from domain.ports import TemplateRepository

logger = logging.getLogger(__name__)


def seed_from_filesystem(template_dir: str, repo: TemplateRepository) -> None:
    """Insert filesystem templates that are not yet in the database.

    Only inserts — never updates. This ensures LLM-refined templates already
    in the DB are not clobbered by the on-disk originals on restart.
    """
    if not os.path.isdir(template_dir):
        logger.info("Template seed skipped: directory '%s' not found", template_dir)
        return

    yml_files = [f for f in os.listdir(template_dir) if f.endswith(".yml")]
    if not yml_files:
        logger.info("Template seed skipped: no .yml files in '%s'", template_dir)
        return

    seeded = 0
    for filename in sorted(yml_files):
        filepath = os.path.join(template_dir, filename)
        try:
            with open(filepath, encoding="utf-8") as f:
                content = f.read()

            issuer: str | None = None
            try:
                parsed = yaml.safe_load(content)
                if isinstance(parsed, dict):
                    issuer = parsed.get("issuer") or None
            except yaml.YAMLError:
                pass  # issuer stays None; template content is still valid

            # Only insert if not already present — never overwrite DB copy
            existing = repo.get_by_name(filename)
            if existing is None:
                repo.save(name=filename, content=content, issuer=issuer)
                seeded += 1
                logger.debug("Seeded template '%s'", filename)
            else:
                logger.debug("Template '%s' already in database, skipping", filename)

        except OSError as exc:
            logger.warning("Could not read template file '%s': %s", filepath, exc)

    logger.info("Template seed complete: %d new template(s) inserted from '%s'", seeded, template_dir)
