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
        echo [!] mpv no encontrado. Instalando automaticamente con winget...
        echo     (Esto puede tardar unos segundos, relajate...)
        echo.
        winget install mpv.net --accept-package-agreements --accept-source-agreements
        if !errorlevel! equ 0 (
            echo.
            echo [OK] mpv instalado con exito.
        ) else (
            echo.
            echo [AVISO] winget no pudo completar la instalacion automatica.
            echo Podras ver partidos, pero si quieres video coloca mpv.exe en esta carpeta.
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
