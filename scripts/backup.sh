#!/usr/bin/env bash
# ==============================================================================
# Go CMS - Automated Disaster Recovery & Backup Script
# ==============================================================================
# Backs up the database (PostgreSQL, MySQL, or MongoDB) and media uploads,
# packaging them into a timestamped, gzip-compressed tar archive with SHA256 checksums.
#
# Usage:
#   ./scripts/backup.sh [output_dir]
# ==============================================================================

set -euo pipefail

# Configuration defaults (can be overridden by environment variables)
DB_DRIVER="${CMS_DB_DRIVER:-postgres}"
DB_HOST="${CMS_DB_HOST:-localhost}"
DB_PORT="${CMS_DB_PORT:-5432}"
DB_NAME="${CMS_DB_NAME:-cms_db}"
DB_USER="${CMS_DB_USER:-cms}"
DB_PASSWORD="${CMS_DB_PASSWORD:-cms_secret}"
UPLOADS_DIR="${CMS_MEDIA_UPLOAD_DIR:-uploads}"

TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_DIR="${1:-./backups}"
TARGET_BACKUP_NAME="cms_backup_${TIMESTAMP}"
WORK_DIR="${BACKUP_DIR}/${TARGET_BACKUP_NAME}"

echo "========================================================"
echo " Starting Go CMS Disaster Recovery Backup"
echo " Timestamp: ${TIMESTAMP}"
echo " Database Driver: ${DB_DRIVER}"
echo " Uploads Dir: ${UPLOADS_DIR}"
echo " Target Archive: ${BACKUP_DIR}/${TARGET_BACKUP_NAME}.tar.gz"
echo "========================================================"

mkdir -p "${WORK_DIR}"

# 1. Database Dump
case "${DB_DRIVER}" in
  postgres)
    echo "--> Dumping PostgreSQL database '${DB_NAME}'..."
    PGPASSWORD="${DB_PASSWORD}" pg_dump \
      -h "${DB_HOST}" \
      -p "${DB_PORT}" \
      -U "${DB_USER}" \
      -d "${DB_NAME}" \
      -F c \
      -b -v \
      -f "${WORK_DIR}/database.dump" || {
        echo "WARNING: pg_dump failed or not in PATH, attempting plain SQL dump..."
        PGPASSWORD="${DB_PASSWORD}" pg_dump \
          -h "${DB_HOST}" \
          -p "${DB_PORT}" \
          -U "${DB_USER}" \
          -d "${DB_NAME}" > "${WORK_DIR}/database.sql"
      }
    ;;

  mysql)
    echo "--> Dumping MySQL database '${DB_NAME}'..."
    MYSQL_PWD="${DB_PASSWORD}" mysqldump \
      -h "${DB_HOST}" \
      -P "${DB_PORT}" \
      -u "${DB_USER}" \
      --single-transaction \
      --quick \
      "${DB_NAME}" > "${WORK_DIR}/database.sql"
    ;;

  mongodb)
    echo "--> Dumping MongoDB database '${DB_NAME}'..."
    mongodump \
      --host="${DB_HOST}:${DB_PORT}" \
      --db="${DB_NAME}" \
      --out="${WORK_DIR}/mongo_dump"
    ;;

  *)
    echo "ERROR: Unsupported database driver: ${DB_DRIVER}"
    exit 1
    ;;
esac

# 2. Media Uploads Archive
if [ -d "${UPLOADS_DIR}" ]; then
  echo "--> Archiving media assets from '${UPLOADS_DIR}'..."
  tar -czf "${WORK_DIR}/uploads.tar.gz" -C "$(dirname "${UPLOADS_DIR}")" "$(basename "${UPLOADS_DIR}")"
else
  echo "--> Notice: Uploads directory '${UPLOADS_DIR}' does not exist, creating empty archive."
  tar -czf "${WORK_DIR}/uploads.tar.gz" --files-from /dev/null
fi

# 3. Metadata Manifest
cat <<EOF > "${WORK_DIR}/manifest.json"
{
  "version": "1.0.0",
  "created_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "database_driver": "${DB_DRIVER}",
  "database_name": "${DB_NAME}",
  "backup_name": "${TARGET_BACKUP_NAME}"
}
EOF

# 4. Packaging Archive & Generating Checksums
echo "--> Compressing complete backup bundle..."
cd "${BACKUP_DIR}"
tar -czf "${TARGET_BACKUP_NAME}.tar.gz" "${TARGET_BACKUP_NAME}"
rm -rf "${TARGET_BACKUP_NAME}"

# Generate SHA-256 Checksum
if command -v sha256sum &> /dev/null; then
  sha256sum "${TARGET_BACKUP_NAME}.tar.gz" > "${TARGET_BACKUP_NAME}.tar.gz.sha256"
elif command -v shasum &> /dev/null; then
  shasum -a 256 "${TARGET_BACKUP_NAME}.tar.gz" > "${TARGET_BACKUP_NAME}.tar.gz.sha256"
fi

echo "========================================================"
echo " Backup successfully created:"
echo " Archive:  ${BACKUP_DIR}/${TARGET_BACKUP_NAME}.tar.gz"
if [ -f "${BACKUP_DIR}/${TARGET_BACKUP_NAME}.tar.gz.sha256" ]; then
  echo " Checksum: $(cat "${BACKUP_DIR}/${TARGET_BACKUP_NAME}.tar.gz.sha256")"
fi
echo "========================================================"

