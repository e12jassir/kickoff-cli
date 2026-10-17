@echo off
setlocal enabledelayedexpansion
title Kickoff - Instalador de MPV
echo ========================================================
echo   Kickoff - Instalando reproductor de video (mpv)
echo ========================================================
echo.

set "SCRIPT_DIR=%~dp0"

echo Descargando e instalando reproductor portable (mpv.net)...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference = 'Stop'; " ^
  "$dir = '%SCRIPT_DIR%'; " ^
  "$dest = Join-Path $dir 'mpv_portable.zip'; " ^
  "try { " ^
  "  $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/mpvnet-player/mpv.net/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing; " ^
  "  $asset = $rel.assets | Where-Object { $_.name -match 'portable-x64\.zip$' } | Select-Object -First 1; " ^
  "  if (-not $asset) { throw 'No se encontro release portable' }; " ^
  "  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest -UseBasicParsing; " ^
  "  $temp = Join-Path $dir 'mpv_temp'; " ^
  "  if (Test-Path $temp) { Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue }; " ^
  "  Expand-Archive -Path $dest -DestinationPath $temp -Force; " ^
  "  Get-ChildItem -Path $temp | ForEach-Object { Copy-Item $_.FullName -Destination $dir -Recurse -Force }; " ^
  "  Remove-Item $dest -Force -ErrorAction SilentlyContinue; " ^
  "  Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue; " ^
  "  if (Test-Path (Join-Path $dir 'mpvnet.exe')) { Copy-Item (Join-Path $dir 'mpvnet.exe') (Join-Path $dir 'mpv.exe') -Force }; " ^
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
    winget install mpv.net --accept-package-agreements --accept-source-agreements
    if %errorlevel% equ 0 (
        echo [OK] mpv instalado via winget.
    ) else (
        echo [!] No se pudo instalar automaticamente.
        echo     Descarga mpv desde https://mpv.io o coloca mpv.exe en esta carpeta.
    )
)
echo.
pause
