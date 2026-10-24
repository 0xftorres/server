# Compila (si hace falta) y levanta la API en primer plano.
#
#   .\run.ps1                 # http://localhost:5090/api  (lee .env si existe)
#   .\run.ps1 -Port 8080
#   .\run.ps1 -Release
param(
    [switch]$Release,
    [int]$Port = 0
)

$ErrorActionPreference = 'Continue'  # cargo escribe en stderr; se controla $LASTEXITCODE
Set-Location $PSScriptRoot

& "$PSScriptRoot\build.ps1" -Release:$Release
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($Port -gt 0) { $env:PORT = "$Port" }
$profile = if ($Release) { 'release' } else { 'debug' }
& "$PSScriptRoot\target\$profile\tenis-ahora-api.exe"
