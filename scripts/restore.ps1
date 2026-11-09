# ==============================================================================
# Go CMS - Automated Disaster Recovery & Restore Script (PowerShell)
# ==============================================================================
# Verifies SHA256 integrity, extracts archive, and restores database and uploads.
#
# Usage:
#   .\scripts\restore.ps1 -Archive ".\backups\cms_backup_20260101_120000.zip" [-Force]
# ==============================================================================

param (
    [Parameter(Mandatory=$true)]
    [string]$Archive,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Archive)) {
    Write-Error "Backup archive not found at path: $Archive"
}

# 1. Verify Checksum if present
$hashPath = "$Archive.sha256"
if (Test-Path $hashPath) {
    Write-Host "--> Verifying SHA256 checksum..." -ForegroundColor Green
    $expectedHash = ((Get-Content $hashPath).Trim() -split "\s+")[0].ToUpper()
    $actualHash = (Get-FileHash -Path $Archive -Algorithm SHA256).Hash.ToUpper()

    if ($expectedHash -ne $actualHash) {
        Write-Error "SHA256 checksum mismatch! Archive may be corrupted or modified."
    }
    Write-Host "✓ SHA256 checksum verified successfully." -ForegroundColor Green
}

if (-not $Force) {
    $confirm = Read-Host "WARNING: Restoring will overwrite existing media and database content. Proceed? (y/N)"
    if ($confirm -notmatch "^[yY](es)?$") {
        Write-Host "Restore cancelled by user."
        exit 0
    }
}

$tempExtract = Join-Path $env:TEMP ("cms_restore_" + (Get-Random))
New-Item -ItemType Directory -Path $tempExtract -Force | Out-Null

try {
    Write-Host "--> Extracting archive to temporary directory..." -ForegroundColor Green
    Expand-Archive -Path $Archive -DestinationPath $tempExtract -Force

    $DB_DRIVER = if ($env:CMS_DB_DRIVER) { $env:CMS_DB_DRIVER } else { "postgres" }
    $DB_HOST   = if ($env:CMS_DB_HOST) { $env:CMS_DB_HOST } else { "localhost" }
    $DB_PORT   = if ($env:CMS_DB_PORT) { $env:CMS_DB_PORT } else { "5432" }
    $DB_NAME   = if ($env:CMS_DB_NAME) { $env:CMS_DB_NAME } else { "cms_db" }
    $DB_USER   = if ($env:CMS_DB_USER) { $env:CMS_DB_USER } else { "cms" }
    $DB_PASS   = if ($env:CMS_DB_PASSWORD) { $env:CMS_DB_PASSWORD } else { "cms_secret" }
    $UPLOADS_DIR = if ($env:CMS_MEDIA_UPLOAD_DIR) { $env:CMS_MEDIA_UPLOAD_DIR } else { "uploads" }

    # Restore Database
    $sqlFile = Join-Path $tempExtract "database.sql"
    if (Test-Path $sqlFile) {
        Write-Host "--> Restoring database from database.sql..." -ForegroundColor Green
        switch ($DB_DRIVER.ToLower()) {
            "postgres" {
                $env:PGPASSWORD = $DB_PASS
                if (Get-Command psql -ErrorAction SilentlyContinue) {
                    & psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f $sqlFile
                    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL restore failed with exit code $LASTEXITCODE" }
                } else {
                    throw "psql command not found in PATH."
                }
            }
            "mysql" {
                if (Get-Command mysql -ErrorAction SilentlyContinue) {
                    Get-Content $sqlFile | & mysql -h $DB_HOST -P $DB_PORT -u $DB_USER -p$DB_PASS $DB_NAME
                    if ($LASTEXITCODE -ne 0) { throw "MySQL restore failed with exit code $LASTEXITCODE" }
                } else {
                    throw "mysql command not found in PATH."
                }
            }
        }
    }

    # Restore Uploads
    $extractUploads = Join-Path $tempExtract "uploads"
    if (Test-Path $extractUploads) {
        Write-Host "--> Restoring uploads to '$UPLOADS_DIR'..." -ForegroundColor Green
        if (-not (Test-Path $UPLOADS_DIR)) {
            New-Item -ItemType Directory -Path $UPLOADS_DIR -Force | Out-Null
        }
        Copy-Item -Path "$extractUploads\*" -Destination $UPLOADS_DIR -Recurse -Force
    }

    Write-Host "========================================================" -ForegroundColor Cyan
    Write-Host " Restore completed successfully!" -ForegroundColor Green
    Write-Host "========================================================" -ForegroundColor Cyan
}
finally {
    if (Test-Path $tempExtract) {
        Remove-Item -Path $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    }
}
