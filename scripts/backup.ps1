# ==============================================================================
# Go CMS - Automated Disaster Recovery & Backup Script (PowerShell)
# ==============================================================================
# Backs up the database and media uploads into a timestamped zip archive with SHA256.
#
# Usage:
#   .\scripts\backup.ps1 [-BackupDir ".\backups"]
# ==============================================================================

param (
    [string]$BackupDir = ".\backups"
)

$ErrorActionPreference = "Stop"

$DB_DRIVER = if ($env:CMS_DB_DRIVER) { $env:CMS_DB_DRIVER } else { "postgres" }
$DB_HOST   = if ($env:CMS_DB_HOST) { $env:CMS_DB_HOST } else { "localhost" }
$DB_PORT   = if ($env:CMS_DB_PORT) { $env:CMS_DB_PORT } else { "5432" }
$DB_NAME   = if ($env:CMS_DB_NAME) { $env:CMS_DB_NAME } else { "cms_db" }
$DB_USER   = if ($env:CMS_DB_USER) { $env:CMS_DB_USER } else { "cms" }
$DB_PASS   = if ($env:CMS_DB_PASSWORD) { $env:CMS_DB_PASSWORD } else { "cms_secret" }
$UPLOADS_DIR = if ($env:CMS_MEDIA_UPLOAD_DIR) { $env:CMS_MEDIA_UPLOAD_DIR } else { "uploads" }

$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$targetName = "cms_backup_$timestamp"
$workDir = Join-Path $BackupDir $targetName

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host " Starting Go CMS Disaster Recovery Backup (PowerShell)" -ForegroundColor Cyan
Write-Host " Timestamp: $timestamp"
Write-Host " Database Driver: $DB_DRIVER"
Write-Host " Uploads Directory: $UPLOADS_DIR"
Write-Host " Target Backup: $workDir.zip"
Write-Host "========================================================" -ForegroundColor Cyan

if (-not (Test-Path $workDir)) {
    New-Item -ItemType Directory -Path $workDir -Force | Out-Null
}

# 1. Database Dump
switch ($DB_DRIVER.ToLower()) {
    "postgres" {
        Write-Host "--> Dumping PostgreSQL database '$DB_NAME'..." -ForegroundColor Green
        $env:PGPASSWORD = $DB_PASS
        $sqlPath = Join-Path $workDir "database.sql"
        if (Get-Command pg_dump -ErrorAction SilentlyContinue) {
            & pg_dump -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f $sqlPath
        } else {
            Write-Warning "pg_dump command not found in PATH; skipping database dump."
        }
    }
    "mysql" {
        Write-Host "--> Dumping MySQL database '$DB_NAME'..." -ForegroundColor Green
        $sqlPath = Join-Path $workDir "database.sql"
        if (Get-Command mysqldump -ErrorAction SilentlyContinue) {
            & mysqldump -h $DB_HOST -P $DB_PORT -u $DB_USER -p$DB_PASS --single-transaction $DB_NAME > $sqlPath
        } else {
            Write-Warning "mysqldump command not found in PATH; skipping database dump."
        }
    }
    default {
        Write-Warning "Database driver '$DB_DRIVER' does not have built-in dump support."
    }
}

# 2. Archive Uploads
$workUploads = Join-Path $workDir "uploads"
if (Test-Path $UPLOADS_DIR) {
    Write-Host "--> Copying media uploads from '$UPLOADS_DIR'..." -ForegroundColor Green
    Copy-Item -Path $UPLOADS_DIR -Destination $workUploads -Recurse -Force
} else {
    New-Item -ItemType Directory -Path $workUploads -Force | Out-Null
}

# 3. Create Manifest
$manifest = @{
    version = "1.0.0"
    created_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    database_driver = $DB_DRIVER
    database_name = $DB_NAME
    backup_name = $targetName
} | ConvertTo-Json
$manifest | Out-File -FilePath (Join-Path $workDir "manifest.json") -Encoding utf8

# 4. Compress to Zip
$zipPath = "$workDir.zip"
Write-Host "--> Compressing backup bundle to '$zipPath'..." -ForegroundColor Green
Compress-Archive -Path "$workDir\*" -DestinationPath $zipPath -Force
Remove-Item -Path $workDir -Recurse -Force

# 5. Calculate SHA256 Checksum
$hash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
$hashPath = "$zipPath.sha256"
"$hash  $(Split-Path $zipPath -Leaf)" | Out-File -FilePath $hashPath -Encoding ascii

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host " Backup successfully created:" -ForegroundColor Green
Write-Host " Archive:  $zipPath"
Write-Host " SHA-256:  $hash"
Write-Host "========================================================" -ForegroundColor Cyan

