#!/usr/bin/env bash
# Dumps the platform's PostgreSQL metadata database to a timestamped,
# gzip-compressed custom-format archive (§33 disaster recovery). This backs
# up platform metadata only (orgs/projects/clusters/workflows/audit/etc.) —
# not managed-cluster desired state, which lives in Git (ADR-0004).
#
# Usage:
#   PLATFORM_POSTGRES_DSN=postgres://user:pass@host:5432/platform \
#     ./scripts/backup-postgres.sh [output-dir]
set -euo pipefail

: "${PLATFORM_POSTGRES_DSN:?PLATFORM_POSTGRES_DSN must be set (see docs/getting-started)}"

out_dir="${1:-./backups}"
mkdir -p "${out_dir}"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
out_file="${out_dir}/platform-${timestamp}.dump"

echo "Backing up platform database to ${out_file}" >&2
pg_dump --dbname="${PLATFORM_POSTGRES_DSN}" --format=custom --compress=9 --file="${out_file}"

echo "${out_file}"
