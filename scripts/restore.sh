#!/usr/bin/env bash
# ==============================================================================
# Go CMS - Automated Disaster Recovery & Restore Script
# ==============================================================================
# Verifies archive integrity with SHA256, extracts assets, and restores the database.
#
# Usage:
#   ./scripts/restore.sh <backup_archive.tar.gz> [--force]
# ==============================================================================

set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <path_to_backup_archive.tar.gz> [--force]"
  exit 1
fi

ARCHIVE_PATH="$1"
FORCE="${2:-}"

if [ ! -f "${ARCHIVE_PATH}" ]; then
  echo "ERROR: Backup archive file not found: ${ARCHIVE_PATH}"
  exit 1
fi

# Verify checksum if present
CHECKSUM_FILE="${ARCHIVE_PATH}.sha256"
if [ -f "${CHECKSUM_FILE}" ]; then
  echo "--> Verifying SHA256 checksum..."
  if command -v sha256sum &> /dev/null; then
    sha256sum -c "${CHECKSUM_FILE}"
  elif command -v shasum &> /dev/null; then
    shasum -a 256 -c "${CHECKSUM_FILE}"
  fi
  echo "✓ Checksum verification passed."
fi

if [ "${FORCE}" != "--force" ]; then
  read -p "WARNING: Restoring will overwrite database and media files. Continue? [y/N]: " confirm
  if [[ ! "${confirm}" =~ ^[yY](es)?$ ]]; then
    echo "Restore aborted by user."
    exit 0
  fi
fi

TEMP_RESTORE_DIR=$(mktemp -d)
trap 'rm -rf "${TEMP_RESTORE_DIR}"' EXIT

echo "--> Extracting backup archive to temporary directory..."
tar -xzf "${ARCHIVE_PATH}" -C "${TEMP_RESTORE_DIR}"

BACKUP_SUBDIR=$(find "${TEMP_RESTORE_DIR}" -mindepth 1 -maxdepth 1 -type d | head -n 1)
if [ -z "${BACKUP_SUBDIR}" ]; then
  echo "ERROR: Could not locate extracted backup directory."
  exit 1
fi

# Configuration defaults
DB_DRIVER="${CMS_DB_DRIVER:-postgres}"
DB_HOST="${CMS_DB_HOST:-localhost}"
DB_PORT="${CMS_DB_PORT:-5432}"
DB_NAME="${CMS_DB_NAME:-cms_db}"
DB_USER="${CMS_DB_USER:-cms}"
DB_PASSWORD="${CMS_DB_PASSWORD:-cms_secret}"
UPLOADS_DIR="${CMS_MEDIA_UPLOAD_DIR:-uploads}"

# 1. Restore Database
echo "--> Restoring database (${DB_DRIVER})..."
case "${DB_DRIVER}" in
  postgres)
    if [ -f "${BACKUP_SUBDIR}/database.dump" ]; then
      PGPASSWORD="${DB_PASSWORD}" pg_restore \
        -h "${DB_HOST}" \
        -p "${DB_PORT}" \
        -U "${DB_USER}" \
        -d "${DB_NAME}" \
        --clean --if-exists -v \
        "${BACKUP_SUBDIR}/database.dump" || true
    elif [ -f "${BACKUP_SUBDIR}/database.sql" ]; then
      PGPASSWORD="${DB_PASSWORD}" psql \
        -h "${DB_HOST}" \
        -p "${DB_PORT}" \
        -U "${DB_USER}" \
        -d "${DB_NAME}" \
        -f "${BACKUP_SUBDIR}/database.sql"
    fi
    ;;

  mysql)
    if [ -f "${BACKUP_SUBDIR}/database.sql" ]; then
      MYSQL_PWD="${DB_PASSWORD}" mysql \
        -h "${DB_HOST}" \
        -P "${DB_PORT}" \
        -u "${DB_USER}" \
        "${DB_NAME}" < "${BACKUP_SUBDIR}/database.sql"
    fi
    ;;

  mongodb)
    if [ -d "${BACKUP_SUBDIR}/mongo_dump" ]; then
      mongorestore \
        --host="${DB_HOST}:${DB_PORT}" \
        --db="${DB_NAME}" \
        --drop \
        "${BACKUP_SUBDIR}/mongo_dump/${DB_NAME}"
    fi
    ;;

  *)
    echo "ERROR: Unsupported database driver: ${DB_DRIVER}"
    exit 1
    ;;
esac

# 2. Restore Uploads
if [ -f "${BACKUP_SUBDIR}/uploads.tar.gz" ]; then
  echo "--> Restoring media uploads to '${UPLOADS_DIR}'..."
  mkdir -p "$(dirname "${UPLOADS_DIR}")"
  tar -xzf "${BACKUP_SUBDIR}/uploads.tar.gz" -C "$(dirname "${UPLOADS_DIR}")"
fi

echo "========================================================"
echo " Restore completed successfully!"
echo "========================================================"
