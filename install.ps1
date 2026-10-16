# Kickoff CLI Universal 1-Click Installer for Windows
# Usage: powershell -c "irm https://raw.githubusercontent.com/e12jassir/kickoff-cli/main/install.ps1 | iex"

[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$Version,

    [Parameter(Mandatory = $false)]
    [switch]$Yes
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = "e12jassir/kickoff-cli"

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host "             KICKOFF CLI - WINDOWS INSTALLER           " -ForegroundColor Cyan
Write-Host "========================================================" -ForegroundColor Cyan
Write-Host ""

# 1. Architecture Detection
$arch = $env:PROCESSOR_ARCHITECTURE
$arch64 = $env:PROCESSOR_ARCHITEW6432

if ($arch -eq "ARM64" -or $arch64 -eq "ARM64") {
    $archName = "arm64"
} else {
    $archName = "amd64"
}

Write-Host "[1/4] Detected system architecture: $archName"

# 2. Installation Directory Setup
$installDir = Join-Path $env:LOCALAPPDATA "Programs\kickoff"
if (-not (Test-Path -Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# Add to User PATH if missing
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($null -eq $userPath) {
    $userPath = ""
}

$pathEntries = $userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if ($pathEntries -notcontains $installDir) {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { "$userPath;$installDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$installDir"
    Write-Host "[OK] Added $installDir to User PATH."
} else {
    Write-Host "[OK] $installDir is already in User PATH."
}

# 3. Download and Extract Release
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("kickoff_install_" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$zipPath = Join-Path $tempDir "kickoff_windows.zip"
$extractPath = Join-Path $tempDir "extracted"

if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $cleanVer = $Version.TrimStart('v')
    $tag = "v$cleanVer"
    Write-Host "[2/4] Downloading Kickoff $tag for windows/$archName..."
    $downloadUrls = @(
        "https://github.com/$Repo/releases/download/$tag/kickoff_windows_$archName.zip",
        "https://github.com/$Repo/releases/download/$tag/kickoff_${cleanVer}_windows_$archName.zip"
    )
} else {
    Write-Host "[2/4] Downloading latest Kickoff release for windows/$archName..."
    $downloadUrls = @(
        "https://github.com/$Repo/releases/latest/download/kickoff_windows_$archName.zip",
        "https://github.com/$Repo/releases/latest/download/kickoff_0.1.2_windows_$archName.zip"
    )
}

try {
    $downloaded = $false
    foreach ($url in $downloadUrls) {
        try {
            Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
            if (Test-Path -Path $zipPath) {
                $downloaded = $true
                break
            }
        } catch {
            # Try next candidate URL
        }
    }

    if (-not $downloaded) {
        throw "Failed to download Kickoff release asset. Check https://github.com/$Repo/releases for available builds."
    }

    Expand-Archive -Path $zipPath -DestinationPath $extractPath -Force

    $extractedExe = Get-ChildItem -Path $extractPath -Filter "kickoff.exe" -Recurse | Select-Object -First 1
    if (-not $extractedExe) {
        throw "kickoff.exe was not found inside the downloaded archive."
    }

    $targetExe = Join-Path $installDir "kickoff.exe"
    Copy-Item -Path $extractedExe.FullName -Destination $targetExe -Force

    # Copy icon if included in the release
    $extractedIco = Get-ChildItem -Path $extractPath -Filter "kickoff.ico" -Recurse | Select-Object -First 1
    if ($extractedIco) {
        Copy-Item -Path $extractedIco.FullName -Destination (Join-Path $installDir "kickoff.ico") -Force
    }

    Write-Host "[OK] kickoff.exe extracted and installed successfully."
} finally {
    if (Test-Path -Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 4. Check Video Playback Dependency (mpv)
Write-Host "[3/4] Checking video player dependencies..."
$hasMpv = (Get-Command mpv.exe -ErrorAction SilentlyContinue) -or (Get-Command mpvnet.exe -ErrorAction SilentlyContinue)
if (-not $hasMpv) {
    $hasWinget = Get-Command winget.exe -ErrorAction SilentlyContinue
    if ($hasWinget) {
        $installMpv = $false
        if ($Yes) {
            $installMpv = $true
        } else {
            $response = Read-Host "mpv is recommended for video playback. Install mpv.net via winget? [Y/n]"
            if ([string]::IsNullOrWhiteSpace($response) -or $response -match '^[Yy]') {
                $installMpv = $true
            }
        }

        if ($installMpv) {
            Write-Host "Installing mpv.net via winget..."
            try {
                Start-Process -FilePath "winget" -ArgumentList "install mpv.net --accept-source-agreements --accept-package-agreements" -NoNewWindow -Wait
                Write-Host "[OK] mpv.net installation completed."
            } catch {
                Write-Warning "Could not install mpv.net automatically via winget: $_"
                Write-Host "You can install mpv manually from https://mpv.io or run 'winget install mpv.net'."
            }
        } else {
            Write-Host "Skipped mpv installation. Note: Kickoff score tracking works without mpv. Video playback requires mpv (https://mpv.io)."
        }
    } else {
        Write-Host "mpv is recommended for video playback. winget was not found; please install mpv manually from https://mpv.io."
    }
} else {
    Write-Host "[OK] Video player (mpv) is already installed."
}

# 5. Import Signals Pack if Configured
if (-not [string]::IsNullOrWhiteSpace($env:KICKOFF_SIGNALS_URL)) {
    Write-Host "[4/4] Importing configured signals pack..."
    $exePath = Join-Path $installDir "kickoff.exe"
    $helpOutput = & $exePath --help 2>&1 | Out-String
    if ($helpOutput -match "config import") {
        try {
            & $exePath config import $env:KICKOFF_SIGNALS_URL
        } catch {
            Write-Warning "Could not import signals pack from $env:KICKOFF_SIGNALS_URL: $_"
        }
    } else {
        Write-Warning "This version of kickoff does not support 'config import'. Skipping signals import."
    }
}

Write-Host ""
Write-Host "========================================================" -ForegroundColor Green
Write-Host "  Kickoff installed successfully!" -ForegroundColor Green
Write-Host "  Location: $(Join-Path $installDir 'kickoff.exe')" -ForegroundColor Green
Write-Host "========================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Open a new terminal or PowerShell window and run 'kickoff' to start."
