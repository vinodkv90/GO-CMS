$distDir = 'd:\Github\Go CMS\dist'
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

$version = '0.1.0'

Write-Host "Building Windows amd64..."
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -ldflags "-s -w" -o (Join-Path $distDir "go-cms.exe") ./cmd/go-cms
Compress-Archive -Path (Join-Path $distDir "go-cms.exe") -DestinationPath (Join-Path $distDir "go-cms_${version}_windows_amd64.zip") -Force
Remove-Item (Join-Path $distDir "go-cms.exe") -Force

Write-Host "Building Linux amd64..."
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -ldflags "-s -w" -o (Join-Path $distDir "go-cms") ./cmd/go-cms
tar -czf (Join-Path $distDir "go-cms_${version}_linux_amd64.tar.gz") -C $distDir go-cms
Remove-Item (Join-Path $distDir "go-cms") -Force

Write-Host "Building Darwin arm64 (Apple Silicon)..."
$env:GOOS = 'darwin'
$env:GOARCH = 'arm64'
go build -ldflags "-s -w" -o (Join-Path $distDir "go-cms") ./cmd/go-cms
tar -czf (Join-Path $distDir "go-cms_${version}_darwin_arm64.tar.gz") -C $distDir go-cms
Remove-Item (Join-Path $distDir "go-cms") -Force

Write-Host "Building Darwin amd64 (Intel Mac)..."
$env:GOOS = 'darwin'
$env:GOARCH = 'amd64'
go build -ldflags "-s -w" -o (Join-Path $distDir "go-cms") ./cmd/go-cms
tar -czf (Join-Path $distDir "go-cms_${version}_darwin_amd64.tar.gz") -C $distDir go-cms
Remove-Item (Join-Path $distDir "go-cms") -Force

Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

# Compute checksums.txt
$files = Get-ChildItem -Path $distDir -Filter "go-cms_*"
$checksumLines = @()
foreach ($file in $files) {
    $sha = (Get-FileHash -Path $file.FullName -Algorithm SHA256).Hash.ToLower()
    $checksumLines += ($sha + "  " + $file.Name)
}
$checksumLines | Set-Content (Join-Path $distDir "checksums.txt")

Write-Host "Generated release artifacts in $($distDir):"
Get-ChildItem $distDir
Write-Host ""
Write-Host "Checksums:"
Get-Content (Join-Path $distDir "checksums.txt")
