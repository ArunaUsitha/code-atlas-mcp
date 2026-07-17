# Build script for Windows (PowerShell)
$env:PATH = "$PWD\.gcc\w64devkit\bin;$env:PATH"
$env:CGO_ENABLED = "1"
Write-Host "Building CodeAtlas MCP Server..." -ForegroundColor Cyan
go build -o cbm-server.exe ./cmd/cbm-server
if ($LASTEXITCODE -eq 0) {
    Write-Host "Build Succeeded! cbm-server.exe is ready." -ForegroundColor Green
} else {
    Write-Host "Build Failed." -ForegroundColor Red
}
