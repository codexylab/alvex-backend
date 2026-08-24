# ALVEX Backend — Easy Startup Script
# Double-click karo ya PowerShell mein run karo: .\start.ps1

$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$env:GOFLAGS = "-mod=mod"

Write-Host ""
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "   ALVEX Backend Server Starting..." -ForegroundColor Cyan  
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host ""

# Check Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: Go not found at C:\Program Files\Go\bin" -ForegroundColor Red
    Write-Host "Please install Go from https://go.dev/dl/" -ForegroundColor Yellow
    Read-Host "Press Enter to exit"
    exit 1
}

Write-Host "Go version: $(go version)" -ForegroundColor Green

# Go to backend directory
Set-Location $PSScriptRoot

# Run server
Write-Host ""
Write-Host "Starting server on http://localhost:8080 ..." -ForegroundColor Green
Write-Host "Health check: http://localhost:8080/health" -ForegroundColor Gray
Write-Host "Press Ctrl+C to stop" -ForegroundColor Gray
Write-Host ""

go run ./cmd/server/main.go
