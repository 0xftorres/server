# Compila la API. Agrega al PATH el toolchain MinGW-w64 de D:\fede\tools\mingw64 (gcc, ld, as,
# dlltool), necesario para enlazar las crates `windows-sys` con el toolchain x86_64-pc-windows-gnu.
#
#   .\build.ps1            # debug
#   .\build.ps1 -Release   # release (target\release\tenis-ahora-api.exe)
param([switch]$Release)

$ErrorActionPreference = 'Continue'  # cargo escribe en stderr; se controla $LASTEXITCODE
Set-Location $PSScriptRoot

# Toolchain MinGW-w64 (WinLibs) descomprimido en D:\fede\tools\mingw64: aporta gcc, ld, as y
# dlltool consistentes entre sí. Las crates `windows-sys` necesitan dlltool/as al enlazar con
# el toolchain x86_64-pc-windows-gnu de rustup, y el ld de rustup no es compatible con los
# import libs que genera un dlltool más nuevo, por eso se usa el toolchain completo.
# Si MINGW_BIN está definido se usa esa carpeta.
$mingw = if ($env:MINGW_BIN) { $env:MINGW_BIN } else { Join-Path (Split-Path $PSScriptRoot -Parent) 'tools\mingw64\bin' }
if (Test-Path (Join-Path $mingw 'dlltool.exe')) {
    $env:PATH = "$mingw;$env:PATH"
} else {
    Write-Warning "No se encontró MinGW-w64 en $mingw. Descargar WinLibs (https://winlibs.com) y descomprimirlo en D:\fede\tools\ o definir MINGW_BIN."
}

if ($Release) { cargo build --release } else { cargo build }
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Copia la DLL de SQLite junto al ejecutable (por si build.rs no corrió en este perfil).
$profile = if ($Release) { 'release' } else { 'debug' }
$dst = Join-Path $PSScriptRoot "target\$profile\sqlite3.dll"
if (-not (Test-Path $dst)) { Copy-Item (Join-Path $PSScriptRoot 'lib\sqlite3.dll') $dst }
Write-Host "OK: target\$profile\tenis-ahora-api.exe"
