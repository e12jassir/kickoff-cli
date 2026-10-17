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
        "https://github.com/$Repo/releases/download/$tag/kickoff_${cleanVer}_windows_$archName.zip",
        "https://github.com/$Repo/releases/download/$tag/kickoff_windows_$archName.zip"
    )
} else {
    try {
        $latestRel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing
        $tag = $latestRel.tag_name
        $cleanVer = $tag.TrimStart('v')
    } catch {
        $tag = "latest"
        $cleanVer = ""
    }
    Write-Host "[2/4] Downloading latest Kickoff release ($tag) for windows/$archName..."
    $downloadUrls = @(
        "https://github.com/$Repo/releases/download/$tag/kickoff_${cleanVer}_windows_$archName.zip",
        "https://github.com/$Repo/releases/latest/download/kickoff_${cleanVer}_windows_$archName.zip",
        "https://github.com/$Repo/releases/latest/download/kickoff_windows_$archName.zip"
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

    # Copy and register JetBrainsMono Nerd Font if bundled in the release
    $extractedFont = Get-ChildItem -Path $extractPath -Filter "*JetBrainsMono*Nerd*.ttf" -Recurse | Select-Object -First 1
    if ($extractedFont) {
        try {
            $userFontsDir = Join-Path $env:LOCALAPPDATA "Microsoft\Windows\Fonts"
            if (-not (Test-Path -Path $userFontsDir)) {
                New-Item -ItemType Directory -Path $userFontsDir -Force | Out-Null
            }
            $targetFont = Join-Path $userFontsDir $extractedFont.Name
            Copy-Item -Path $extractedFont.FullName -Destination $targetFont -Force
            $regKey = "HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Fonts"
            New-ItemProperty -Path $regKey -Name "JetBrainsMono Nerd Font (TrueType)" -Value $targetFont -PropertyType String -Force | Out-Null
            Write-Host "[OK] JetBrains Mono Nerd Font installed and registered."
        } catch {
            Write-Warning "Could not register font: $_"
        }
    }

    Write-Host "[OK] kickoff.exe extracted and installed successfully."
} finally {
    if (Test-Path -Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 4. Check Video Playback Dependency (mpv)
Write-Host "[3/4] Checking video player dependencies..."
$hasMpv = (Get-Command mpv.exe -ErrorAction SilentlyContinue) -or (Get-Command mpvnet.exe -ErrorAction SilentlyContinue) -or (Test-Path (Join-Path $installDir "mpv.exe")) -or (Test-Path (Join-Path $installDir "mpvnet.exe"))
if (-not $hasMpv) {
    Write-Host "Resolving video player (mpv) for $archName..."
    $installed = $false

    # Try portable mpv extraction from official mpv-player/mpv zip (37MB native)
    try {
        $zipDest = Join-Path $installDir "mpv_portable.zip"
        $rel = Invoke-RestMethod -Uri 'https://api.github.com/repos/mpv-player/mpv/releases/latest' -Headers @{ 'User-Agent' = 'kickoff-cli-installer' } -UseBasicParsing
        $pattern = if ($archName -eq 'arm64') { 'aarch64.*windows-msvc\.zip$' } else { 'x86_64.*windows-msvc\.zip$' }
        $asset = $rel.assets | Where-Object { $_.name -match $pattern } | Select-Object -First 1
        if ($asset) {
            Write-Host "Downloading official mpv player ($($asset.name))..."
            $ProgressPreference = 'SilentlyContinue'
            (New-Object System.Net.WebClient).DownloadFile($asset.browser_download_url, $zipDest)
            $tempMpvDir = Join-Path $installDir "mpv_extracted"
            if (Test-Path $tempMpvDir) { Remove-Item $tempMpvDir -Recurse -Force -ErrorAction SilentlyContinue }
            Expand-Archive -Path $zipDest -DestinationPath $tempMpvDir -Force
            Get-ChildItem -Path $tempMpvDir | ForEach-Object {
                Copy-Item -Path $_.FullName -Destination $installDir -Recurse -Force
            }
            Remove-Item $zipDest -Force -ErrorAction SilentlyContinue
            Remove-Item $tempMpvDir -Recurse -Force -ErrorAction SilentlyContinue

            if (Test-Path (Join-Path $installDir "mpv.exe")) {
                $installed = $true
                Write-Host "[OK] Portable mpv installed successfully into $installDir."
            }
        }
    } catch {
        # Continue to winget fallback
    }

    # Fallback to winget if portable extraction did not succeed
    if (-not $installed) {
        $hasWinget = Get-Command winget.exe -ErrorAction SilentlyContinue
        if ($hasWinget) {
            Write-Host "Installing mpv.net via winget..."
            try {
                Start-Process -FilePath "winget" -ArgumentList "install mpv.net --accept-source-agreements --accept-package-agreements" -NoNewWindow -Wait
                if ((Get-Command mpv.exe -ErrorAction SilentlyContinue) -or (Get-Command mpvnet.exe -ErrorAction SilentlyContinue)) {
                    $installed = $true
                    Write-Host "[OK] mpv.net installed successfully via winget."
                }
            } catch {
                Write-Warning "Could not install mpv.net via winget: $_"
            }
        }
    }

    if (-not $installed) {
        Write-Warning "Could not install mpv automatically. Video playback requires mpv (https://mpv.io)."
    }
} else {
    Write-Host "[OK] Video player (mpv) is already installed."
}

# 5. Import Signals Pack
Write-Host "[4/4] Importing signals pack..."
$signalsUrl = $env:KICKOFF_SIGNALS_URL
if ([string]::IsNullOrWhiteSpace($signalsUrl)) {
    $signalsUrl = "https://gist.githubusercontent.com/e12jassir/b9033b26da64a054f7f68fd1cff1954b/raw/channels.json"
}
$exePath = Join-Path $installDir "kickoff.exe"
try {
    & $exePath import $signalsUrl
    $destChannels = Join-Path $installDir "channels.json"
    (New-Object System.Net.WebClient).DownloadFile($signalsUrl, $destChannels)
    Write-Host "[OK] Signals imported and channels configured successfully."
} catch {
    Write-Warning "Could not import signals pack: $_"
}

Write-Host ""
Write-Host "========================================================" -ForegroundColor Green
Write-Host "  Kickoff installed successfully!" -ForegroundColor Green
Write-Host "  Location: $(Join-Path $installDir 'kickoff.exe')" -ForegroundColor Green
Write-Host "========================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Open a new terminal or PowerShell window and run 'kickoff' to start."
