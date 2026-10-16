@echo off
setlocal enabledelayedexpansion
title Kickoff - Instalador

echo ========================================================
echo               KICKOFF - INSTALADOR 1-CLIC
echo ========================================================
echo.

set "SCRIPT_DIR=%~dp0"
set "TARGET_EXE=%SCRIPT_DIR%kickoff.exe"
set "ICON_FILE=%SCRIPT_DIR%kickoff.ico"

if not exist "%TARGET_EXE%" (
    echo [ERROR] No se encontro kickoff.exe en esta carpeta.
    echo Asegurate de haber descomprimido el ZIP completamente.
    pause
    exit /b 1
)

echo [1/3] Verificando reproductor de video (mpv)...
where mpv >nul 2>nul
if %errorlevel% equ 0 (
    echo [OK] mpv ya esta instalado en el sistema.
) else (
    if exist "%SCRIPT_DIR%mpv.exe" (
        echo [OK] mpv.exe portable encontrado en la carpeta.
    ) else (
        echo [!] mpv no encontrado. Resolviendo el ultimo build portable oficial (x86_64)...
        powershell -NoProfile -ExecutionPolicy Bypass -Command ^
          "$ErrorActionPreference = 'Stop'; " ^
          "$dir = '%SCRIPT_DIR%'; " ^
          "$dest = Join-Path $dir 'mpv_temp.7z'; " ^
          "try { " ^
          "  $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/shinchiro/mpv-winbuild-cmake/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing; " ^
          "  $candidates = $rel.assets | Where-Object { $_.name -match 'x86_64' -and $_.name -match '\.7z$' }; " ^
          "  $asset = $candidates | Where-Object { $_.name -notmatch 'v3' } | Select-Object -First 1; " ^
          "  if (-not $asset) { $asset = $candidates | Select-Object -First 1 }; " ^
          "  if (-not $asset) { throw 'No se encontro un asset x86_64 (.7z) en el ultimo release.' }; " ^
          "  Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest -UseBasicParsing; " ^
          "  Write-Host ('[OK] Build portable descargado: ' + $asset.name); " ^
          "  $sevenZip = (Get-Command 7z.exe -ErrorAction SilentlyContinue).Source; " ^
          "  if (-not $sevenZip) { $p7 = Join-Path $env:ProgramFiles '7-Zip\7z.exe'; if (Test-Path $p7) { $sevenZip = $p7 } }; " ^
          "  if (-not $sevenZip) { Write-Host '[!] 7-Zip no disponible: no se puede extraer mpv_temp.7z.'; exit 2 }; " ^
          "  & $sevenZip x $dest ('-o' + $dir) '-y' | Out-Null; " ^
          "  if (Test-Path (Join-Path $dir 'mpv.exe')) { Write-Host '[OK] mpv.exe extraido correctamente.'; exit 0 } else { Write-Host '[!] La extraccion termino sin encontrar mpv.exe.'; exit 3 } " ^
          "} catch { Write-Host ('[!] No se pudo descargar el build portable: ' + $_.Exception.Message); exit 1 }"

        if !errorlevel! equ 0 (
            echo [OK] mpv instalado con exito.
        ) else (
            where winget >nul 2>nul
            if !errorlevel! equ 0 (
                echo [!] Fallback: instalando mpv.net con winget...
                echo     (Esto puede tardar unos segundos, relajate...)
                winget install mpv.net --accept-package-agreements --accept-source-agreements
            )

            where mpv >nul 2>nul
            if !errorlevel! equ 0 (
                echo [OK] mpv instalado con exito.
            ) else (
                if exist "%SCRIPT_DIR%mpv.exe" (
                    echo [OK] mpv.exe configurado en la carpeta local.
                ) else (
                    echo [AVISO] Podras ver todos los resultados, fixtures y alineaciones.
                    echo         Para ver el video en vivo, instala mpv desde https://mpv.io
                    echo         o coloca mpv.exe en esta misma carpeta.
                )
            )
        )
    )
)
echo.

echo [2/3] Creando acceso directo en el Escritorio con logo oficial...

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ws = New-Object -ComObject WScript.Shell; " ^
  "$desktop = [Environment]::GetFolderPath('Desktop'); " ^
  "$s = $ws.CreateShortcut(\"$desktop\Kickoff.lnk\"); " ^
  "$s.TargetPath = '%TARGET_EXE%'; " ^
  "$s.WorkingDirectory = '%SCRIPT_DIR%'; " ^
  "$s.Description = 'Kickoff - Futbol en vivo en tu terminal'; " ^
  "if (Test-Path '%ICON_FILE%') { $s.IconLocation = '%ICON_FILE%' }; " ^
  "$s.Save()"

if %errorlevel% equ 0 (
    echo [OK] Acceso directo 'Kickoff' creado en tu Escritorio con su logo.
) else (
    echo [!] No se pudo crear el acceso directo en el escritorio.
)
echo.

echo [3/3] Iniciando Kickoff...
timeout /t 2 >nul
start "" "%TARGET_EXE%"
exit /b 0
