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

echo [1/5] Verificando reproductor de video (mpv)...
where mpv >nul 2>nul
if %errorlevel% equ 0 (
    echo [OK] mpv ya esta instalado en el sistema.
) else (
    if exist "%SCRIPT_DIR%mpv.exe" (
        echo [OK] mpv.exe portable encontrado en la carpeta.
    ) else (
        echo [!] mpv no encontrado. Descargando build oficial rapido (37MB)...
        powershell -NoProfile -ExecutionPolicy Bypass -Command ^
          "$ErrorActionPreference = 'Stop'; " ^
          "$dir = '%SCRIPT_DIR%'; " ^
          "$dest = Join-Path $dir 'mpv_portable.zip'; " ^
          "try { " ^
          "  $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/mpv-player/mpv/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing; " ^
          "  $asset = $rel.assets | Where-Object { $_.name -match 'x86_64.*windows-msvc\.zip$' } | Select-Object -First 1; " ^
          "  if (-not $asset) { throw 'No se encontro el archivo zip de mpv' }; " ^
          "  $ProgressPreference = 'SilentlyContinue'; " ^
          "  (New-Object System.Net.WebClient).DownloadFile($asset.browser_download_url, $dest); " ^
          "  $temp = Join-Path $dir 'mpv_temp'; " ^
          "  if (Test-Path $temp) { Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue }; " ^
          "  Expand-Archive -Path $dest -DestinationPath $temp -Force; " ^
          "  Get-ChildItem -Path $temp | ForEach-Object { Copy-Item $_.FullName -Destination $dir -Recurse -Force }; " ^
          "  Remove-Item $dest -Force -ErrorAction SilentlyContinue; " ^
          "  Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue; " ^
          "  Write-Host '[OK] mpv.exe portable instalado correctamente.'; " ^
          "  exit 0; " ^
          "} catch { " ^
          "  Write-Host ('[!] Fallback necesario: ' + $_.Exception.Message); " ^
          "  exit 1; " ^
          "}"

        if !errorlevel! equ 0 (
            echo [OK] mpv instalado con exito.
        ) else (
            where winget >nul 2>nul
            if !errorlevel! equ 0 (
                echo [!] Fallback: instalando mpv con winget...
                winget install io.mpv.mpv --accept-package-agreements --accept-source-agreements
            )

            where mpv >nul 2>nul
            if !errorlevel! equ 0 (
                echo [OK] mpv instalado con exito.
            ) else (
                if exist "%SCRIPT_DIR%mpv.exe" (
                    echo [OK] mpv.exe configurado en la carpeta local.
                ) else (
                    echo [AVISO] Podras ver todos los resultados, fixtures y alineaciones.
                    echo         Para ver el video en vivo, coloca mpv.exe en esta misma carpeta.
                )
            )
        )
    )
)
echo.

echo [2/5] Verificando fuente JetBrains Mono Nerd Font...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$font = Get-ChildItem -Path '%SCRIPT_DIR%' -Filter '*JetBrainsMono*Nerd*.ttf' | Select-Object -First 1; " ^
  "if ($font) { " ^
  "  $dir = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'; " ^
  "  if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }; " ^
  "  $target = Join-Path $dir $font.Name; " ^
  "  Copy-Item $font.FullName -Destination $target -Force; " ^
  "  New-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Fonts' -Name 'JetBrainsMono Nerd Font (TrueType)' -Value $target -PropertyType String -Force | Out-Null; " ^
  "  Write-Host '[OK] Fuente JetBrains Mono Nerd Font instalada en el sistema.' " ^
  "}"
echo.

echo [3/5] Configurando canales y senales...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$dir = '%SCRIPT_DIR%'; " ^
  "$dest = Join-Path $dir 'channels.json'; " ^
  "try { " ^
  "  $ProgressPreference = 'SilentlyContinue'; " ^
  "  (New-Object System.Net.WebClient).DownloadFile('https://gist.githubusercontent.com/e12jassir/b9033b26da64a054f7f68fd1cff1954b/raw/channels.json', $dest); " ^
  "  Write-Host '[OK] Canales configurados con exito en channels.json.'; " ^
  "} catch { Write-Host ('[!] No se pudieron descargar los canales: ' + $_.Exception.Message) }"
echo.

echo [4/5] Creando acceso directo en el Escritorio con logo oficial...

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

echo [5/5] Iniciando Kickoff...
timeout /t 2 >nul
start "" "%TARGET_EXE%"
exit /b 0
