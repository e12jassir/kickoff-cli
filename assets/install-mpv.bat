@echo off
setlocal enabledelayedexpansion
title Kickoff - Instalador de MPV
echo ========================================================
echo   Kickoff - Instalando reproductor de video (mpv)
echo ========================================================
echo.

set "SCRIPT_DIR=%~dp0"

echo Descargando e instalando reproductor rapido (mpv oficial 37MB)...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference = 'Stop'; " ^
  "$dir = '%SCRIPT_DIR%'; " ^
  "$dest = Join-Path $dir 'mpv_portable.zip'; " ^
  "try { " ^
  "  $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/mpv-player/mpv/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing; " ^
  "  $asset = $rel.assets | Where-Object { $_.name -match 'x86_64.*windows-msvc\.zip$' } | Select-Object -First 1; " ^
  "  if (-not $asset) { throw 'No se encontro release zip de mpv' }; " ^
  "  $ProgressPreference = 'SilentlyContinue'; " ^
  "  (New-Object System.Net.WebClient).DownloadFile($asset.browser_download_url, $dest); " ^
  "  $temp = Join-Path $dir 'mpv_temp'; " ^
  "  if (Test-Path $temp) { Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue }; " ^
  "  Expand-Archive -Path $dest -DestinationPath $temp -Force; " ^
  "  Get-ChildItem -Path $temp | ForEach-Object { Copy-Item $_.FullName -Destination $dir -Recurse -Force }; " ^
  "  Remove-Item $dest -Force -ErrorAction SilentlyContinue; " ^
  "  Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue; " ^
  "  Write-Host '[OK] mpv portable instalado con exito en esta carpeta.'; " ^
  "  exit 0; " ^
  "} catch { " ^
  "  exit 1; " ^
  "}"

if %errorlevel% equ 0 (
    echo.
    echo [OK] mpv instalado con exito. Ya puedes ejecutar kickoff.exe!
) else (
    echo [!] Fallback a winget...
    winget install io.mpv.mpv --accept-package-agreements --accept-source-agreements
    if %errorlevel% equ 0 (
        echo [OK] mpv instalado via winget.
    ) else (
        echo [!] No se pudo instalar automaticamente.
        echo     Descarga mpv desde https://mpv.io o coloca mpv.exe en esta carpeta.
    )
)
echo.
pause
