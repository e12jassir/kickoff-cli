@echo off
title Kickoff - Instalador de MPV
echo ========================================================
echo   Kickoff - Instalando reproductor de video (mpv)
echo ========================================================
echo.
winget install mpv.net --accept-package-agreements --accept-source-agreements
if %ERRORLEVEL% EQU 0 (
    echo.
    echo [OK] mpv instalado con exito. Ya puedes ejecutar kickoff.exe!
) else (
    echo.
    echo [!] No se pudo instalar automaticamente via winget.
    echo     Descarga mpv.exe manualmente desde https://mpv.io
    echo     y colocalo en esta misma carpeta junto a kickoff.exe.
)
echo.
pause
