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

# Compute the lowercase SHA-256 hex digest of a file.
function Get-Sha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )
    (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToLower()
}

# Verify a downloaded asset against the release checksums.txt that sits next to it.
# The checksums file is fetched from the SAME base path as the chosen asset.
# Fails closed (throws) on mismatch or missing checksums, unless KICKOFF_SKIP_CHECKSUM=1.
function Assert-AssetChecksum {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(Mandatory = $true)]
        [string]$AssetUrl
    )

    $baseName = $AssetUrl.Substring($AssetUrl.LastIndexOf('/') + 1)
    $checksumsUrl = $AssetUrl.Substring(0, $AssetUrl.LastIndexOf('/')) + '/checksums.txt'
    $checksumsPath = Join-Path (Split-Path -Path $FilePath) 'checksums.txt'
    $skipChecksum = ($env:KICKOFF_SKIP_CHECKSUM -eq '1')

    try {
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing
    } catch {
        Write-Warning "Could not download checksums.txt from: $checksumsUrl"
        if ($skipChecksum) {
            Write-Warning "KICKOFF_SKIP_CHECKSUM=1 is set: skipping integrity verification (proceeding WITHOUT checksum validation)."
            return
        }
        throw "Unable to verify the integrity of the downloaded asset (checksums.txt unavailable). To bypass this check, set `$env:KICKOFF_SKIP_CHECKSUM=1 and retry."
    }

    $expected = $null
    foreach ($line in Get-Content -Path $checksumsPath) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split '\s+', 2
        if ($parts.Count -lt 2) { continue }
        $name = $parts[1].TrimStart('*')
        if ($name -eq $baseName) {
            $expected = $parts[0].ToLower()
            break
        }
    }

    if ([string]::IsNullOrWhiteSpace($expected)) {
        Write-Warning "No checksum entry found for '$baseName' in checksums.txt."
        if ($skipChecksum) {
            Write-Warning "KICKOFF_SKIP_CHECKSUM=1 is set: proceeding WITHOUT a checksum for this asset."
            return
        }
        throw "Refusing to install '$baseName' without a matching checksum entry. To bypass this check, set `$env:KICKOFF_SKIP_CHECKSUM=1 and retry."
    }

    $actual = Get-Sha256 -Path $FilePath
    if ($actual -ne $expected) {
        throw "Checksum mismatch for '$baseName'!`n  expected: $expected`n  actual:   $actual`nThe downloaded file is corrupted or has been tampered with. Aborting."
    }

    Write-Host "[OK] Checksum verified: $baseName"
}

try {
    $downloaded = $false
    $chosenUrl = $null
    foreach ($url in $downloadUrls) {
        try {
            Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
            if (Test-Path -Path $zipPath) {
                $downloaded = $true
                $chosenUrl = $url
                break
            }
        } catch {
            # Try next candidate URL
        }
    }

    if (-not $downloaded) {
        throw "Failed to download Kickoff release asset. Check https://github.com/$Repo/releases for available builds."
    }

    # Verify integrity before extracting or installing (fails closed on mismatch).
    Assert-AssetChecksum -FilePath $zipPath -AssetUrl $chosenUrl

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
        Write-Host "winget not found. Resolving portable mpv build for $archName..."
        try {
            $dest = Join-Path $installDir "mpv_temp.7z"
            $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/shinchiro/mpv-winbuild-cmake/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing
            $candidates = $rel.assets | Where-Object { $_.name -match 'x86_64' -and $_.name -match '\.7z$' }
            $asset = $candidates | Where-Object { $_.name -notmatch 'v3' } | Select-Object -First 1
            if (-not $asset) { $asset = $candidates | Select-Object -First 1 }
            if ($asset) {
                Write-Host "Downloading portable mpv: $($asset.name)..."
                Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest -UseBasicParsing
                $extracted = $false
                $tar = (Get-Command tar.exe -ErrorAction SilentlyContinue).Source
                if ($tar) {
                    & $tar -xf $dest -C $installDir 2>$null
                    if (Test-Path (Join-Path $installDir "mpv.exe")) { $extracted = $true }
                }
                if (-not $extracted) {
                    $sevenZip = (Get-Command 7z.exe -ErrorAction SilentlyContinue).Source
                    if (-not $sevenZip) { $p7 = Join-Path $env:ProgramFiles '7-Zip\7z.exe'; if (Test-Path $p7) { $sevenZip = $p7 } }
                    if ($sevenZip) {
                        & $sevenZip x $dest ('-o' + $installDir) '-y' | Out-Null
                        if (Test-Path (Join-Path $installDir "mpv.exe")) { $extracted = $true }
                    }
                }
                Remove-Item $dest -Force -ErrorAction SilentlyContinue
                if ($extracted) {
                    Write-Host "[OK] Portable mpv installed successfully into $installDir."
                } else {
                    Write-Host "mpv is recommended for video playback. Please install mpv manually from https://mpv.io."
                }
            }
        } catch {
            Write-Host "mpv is recommended for video playback. Please install mpv manually from https://mpv.io."
        }
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
