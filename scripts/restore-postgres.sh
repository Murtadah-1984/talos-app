#!/usr/bin/env bash
# Restores the platform's PostgreSQL metadata database from a dump produced
# by scripts/backup-postgres.sh (§33 disaster recovery). Restores into an
# empty/target database — it does not create the database itself.
#
# Usage:
#   PLATFORM_POSTGRES_DSN=postgres://user:pass@host:5432/platform \
#     ./scripts/restore-postgres.sh /path/to/platform-<timestamp>.dump
set -euo pipefail

: "${PLATFORM_POSTGRES_DSN:?PLATFORM_POSTGRES_DSN must be set (see docs/getting-started)}"

dump_file="${1:?usage: restore-postgres.sh <dump-file>}"
if [[ ! -f "${dump_file}" ]]; then
  echo "dump file not found: ${dump_file}" >&2
  exit 1
fi

echo "Restoring ${dump_file} into ${PLATFORM_POSTGRES_DSN%%\?*}" >&2
pg_restore --dbname="${PLATFORM_POSTGRES_DSN}" --clean --if-exists --no-owner "${dump_file}"

echo "Restore complete. platform-api will re-apply any migrations newer than this snapshot on next startup." >&2
