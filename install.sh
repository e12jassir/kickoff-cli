#!/usr/bin/env bash
set -e

# kickoff-cli universal installer for Linux and macOS
REPO="e12jassir/kickoff-cli"
INSTALL_DIR="$HOME/.local/bin"

SPECIFIED_VERSION=""
AUTO_YES=0

# Parse command line options
while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version)
      if [ -n "$2" ] && [ "${2#-}" = "$2" ]; then
        SPECIFIED_VERSION="$2"
        shift 2
      else
        echo "❌ Error: $1 requires a version value (e.g., --version 0.1.2)" >&2
        exit 1
      fi
      ;;
    --version=*)
      SPECIFIED_VERSION="${1#*=}"
      shift
      ;;
    -y|--yes)
      AUTO_YES=1
      shift
      ;;
    -h|--help)
      echo "Kickoff CLI Universal Installer"
      echo "Usage: install.sh [options]"
      echo ""
      echo "Options:"
      echo "  -v, --version <v>   Install a specific release version (e.g. 0.1.2 or v0.1.2)"
      echo "  -y, --yes           Non-interactive installation (accept dependency prompts automatically)"
      echo "  -h, --help          Show this help message"
      exit 0
      ;;
    *)
      echo "⚠️  Unknown option '$1' ignored." >&2
      shift
      ;;
  esac
done

mkdir -p "$INSTALL_DIR"

# Detect Operating System and Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "❌ Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux)
    OS="linux"
    ;;
  darwin)
    OS="darwin"
    ;;
  *)
    echo "❌ Unsupported operating system: $OS. For Windows, use install.ps1 or download kickoff.exe from releases." >&2
    exit 1
    ;;
esac

# Build download targets
if [ -n "$SPECIFIED_VERSION" ]; then
  CLEAN_VER="${SPECIFIED_VERSION#v}"
  TAG="v${CLEAN_VER}"
  PRIMARY_URL="https://github.com/${REPO}/releases/download/${TAG}/kickoff_${OS}_${ARCH}.tar.gz"
  echo "Installing kickoff-cli ${TAG} for ${OS}/${ARCH}..."
  CANDIDATE_URLS=(
    "$PRIMARY_URL"
    "https://github.com/${REPO}/releases/download/${TAG}/kickoff_${CLEAN_VER}_${OS}_${ARCH}.tar.gz"
    "https://github.com/${REPO}/releases/download/${TAG}/kickoff_macos_${ARCH}.tar.gz"
    "https://github.com/${REPO}/releases/download/${TAG}/kickoff_${CLEAN_VER}_macos_${ARCH}.tar.gz"
  )
else
  PRIMARY_URL="https://github.com/${REPO}/releases/latest/download/kickoff_${OS}_${ARCH}.tar.gz"
  echo "Installing kickoff-cli (latest) for ${OS}/${ARCH}..."
  CANDIDATE_URLS=(
    "$PRIMARY_URL"
    "https://github.com/${REPO}/releases/latest/download/kickoff_macos_${ARCH}.tar.gz"
    "https://github.com/${REPO}/releases/latest/download/kickoff_0.1.2_${OS}_${ARCH}.tar.gz"
    "https://github.com/${REPO}/releases/latest/download/kickoff_0.1.2_macos_${ARCH}.tar.gz"
  )
fi

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

# ==============================================================================
# Download Integrity Verification
# ==============================================================================

# Compute the SHA-256 digest of a file using whatever tool is available:
# sha256sum (Linux) or shasum -a 256 (macOS).
sha256_of() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    return 1
  fi
}

# Verify a downloaded asset against the release checksums.txt that sits next to it.
# The checksums file is fetched from the SAME base path as the chosen asset.
# Returns: 0 = verified, 1 = fatal (do not install), 2 = skipped via explicit opt-out.
verify_checksum() {
  local file="$1"
  local url="$2"
  local base checksums_url checksums_file expected actual

  base="$(basename "$url")"
  checksums_url="$(dirname "$url")/checksums.txt"
  checksums_file="$TEMP_DIR/checksums.txt"

  if ! curl -fsSL -o "$checksums_file" "$checksums_url" 2>/dev/null; then
    echo "⚠️  Warning: could not download checksums.txt from: $checksums_url" >&2
    if [ "${KICKOFF_SKIP_CHECKSUM:-0}" = "1" ]; then
      echo "⚠️  KICKOFF_SKIP_CHECKSUM=1 is set: skipping integrity verification (proceeding WITHOUT checksum validation)." >&2
      return 2
    fi
    echo "❌ Error: unable to verify the integrity of the downloaded asset (checksums.txt unavailable)." >&2
    echo "   Refusing to install an unverified binary. Re-run with KICKOFF_SKIP_CHECKSUM=1 to bypass this check." >&2
    return 1
  fi

  expected="$(awk -v name="$base" '{ n=$2; sub(/^\*/, "", n); if (n == name) { print $1; exit } }' "$checksums_file")"
  if [ -z "$expected" ]; then
    echo "⚠️  Warning: no checksum entry found for '$base' in checksums.txt." >&2
    if [ "${KICKOFF_SKIP_CHECKSUM:-0}" = "1" ]; then
      echo "⚠️  KICKOFF_SKIP_CHECKSUM=1 is set: proceeding WITHOUT a checksum for this asset." >&2
      return 2
    fi
    echo "❌ Error: refusing to install '$base' without a matching checksum entry." >&2
    echo "   Re-run with KICKOFF_SKIP_CHECKSUM=1 to bypass this check." >&2
    return 1
  fi

  if ! actual="$(sha256_of "$file")"; then
    echo "⚠️  Warning: no SHA-256 tool found on this system (need 'sha256sum' or 'shasum')." >&2
    if [ "${KICKOFF_SKIP_CHECKSUM:-0}" = "1" ]; then
      echo "⚠️  KICKOFF_SKIP_CHECKSUM=1 is set: proceeding WITHOUT checksum verification." >&2
      return 2
    fi
    echo "❌ Error: cannot compute SHA-256 to verify the download." >&2
    echo "   Install 'coreutils' (sha256sum) or 'shasum'. Re-run with KICKOFF_SKIP_CHECKSUM=1 to bypass." >&2
    return 1
  fi

  if [ "$actual" != "$expected" ]; then
    echo "❌ Error: checksum mismatch for '$base'!" >&2
    echo "   expected: $expected" >&2
    echo "   actual:   $actual" >&2
    echo "   The downloaded file is corrupted or has been tampered with. Aborting." >&2
    return 1
  fi

  echo "✅ Checksum verified: $base"
  return 0
}

DOWNLOADED=0
for url in "${CANDIDATE_URLS[@]}"; do
  if curl -fsSL -o "$TEMP_DIR/kickoff.tar.gz" "$url" 2>/dev/null; then
    VERIFY_RC=0
    verify_checksum "$TEMP_DIR/kickoff.tar.gz" "$url" || VERIFY_RC=$?
    if [ "$VERIFY_RC" -eq 1 ]; then
      echo "❌ Integrity verification failed for: $url" >&2
      exit 1
    fi
    if tar -xzf "$TEMP_DIR/kickoff.tar.gz" -C "$TEMP_DIR" 2>/dev/null; then
      if [ -f "$TEMP_DIR/kickoff" ]; then
        install -m 755 "$TEMP_DIR/kickoff" "$INSTALL_DIR/kickoff"
        DOWNLOADED=1
        break
      fi
    fi
  fi
done

if [ "$DOWNLOADED" -eq 0 ]; then
  echo "ℹ️  Release asset not found. Building from source via Git..."
  if ! command -v go >/dev/null 2>&1; then
    echo "❌ Error: Go 1.24+ is required to build from source when precompiled assets are not found." >&2
    echo "   Install Go or download release packages from: https://github.com/${REPO}/releases" >&2
    exit 1
  fi
  git clone --depth 1 "https://github.com/${REPO}.git" "$TEMP_DIR/repo"
  (cd "$TEMP_DIR/repo" && go build -ldflags="-s -w" -o "$INSTALL_DIR/kickoff" ./cmd/kickoff)
fi

echo "✅ kickoff installed successfully at: $INSTALL_DIR/kickoff"

# Verify PATH configuration
if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
  echo "⚠️  Note: Ensure $INSTALL_DIR is in your PATH:"
  echo "   export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

# ==============================================================================
# Dependency Verification and Automated Installation
# ==============================================================================

run_elevated() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    if [ -c /dev/tty ]; then
      sudo "$@" </dev/tty
    else
      sudo "$@"
    fi
  else
    return 1
  fi
}

prompt_consent() {
  local prompt_msg="$1"

  if [ "$AUTO_YES" -eq 1 ]; then
    return 0
  fi

  local answer=""
  if [ -c /dev/tty ]; then
    read -r -p "$prompt_msg " answer </dev/tty
  elif [ -t 0 ]; then
    read -r -p "$prompt_msg " answer
  else
    return 1
  fi

  case "$answer" in
    [Yy]|[Yy][Ee][Ss])
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

has_emoji_font() {
  if [ "$OS" = "darwin" ]; then
    return 0 # macOS ships native Apple Color Emoji
  fi
  if command -v fc-list >/dev/null 2>&1; then
    if fc-list : family 2>/dev/null | grep -iq "emoji"; then
      return 0
    fi
  fi
  if [ -d /usr/share/fonts/truetype/noto ] && ls /usr/share/fonts/truetype/noto/*Emoji* >/dev/null 2>&1; then
    return 0
  fi
  if [ -d /usr/share/fonts/noto ] && ls /usr/share/fonts/noto/*Emoji* >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

has_mpv() {
  command -v mpv >/dev/null 2>&1
}

print_manual_instructions() {
  local pm="$1"
  shift
  local pkgs=("$@")

  echo "   You can install missing dependencies manually:"
  case "$pm" in
    apt)    echo "   sudo apt-get update && sudo apt-get install -y ${pkgs[*]}" ;;
    pacman) echo "   sudo pacman -S --needed ${pkgs[*]}" ;;
    dnf)    echo "   sudo dnf install -y ${pkgs[*]}" ;;
    zypper) echo "   sudo zypper install -y ${pkgs[*]}" ;;
    apk)    echo "   sudo apk add ${pkgs[*]}" ;;
    *)      echo "   Install packages: ${pkgs[*]}" ;;
  esac
  echo "   Note: Kickoff core score tracking works without mpv. mpv is only required for video playback."
}

ensure_dependencies() {
  local need_mpv=0
  local need_font=0

  if ! has_mpv; then
    need_mpv=1
  fi

  if ! has_emoji_font; then
    need_font=1
  fi

  if [ "$need_mpv" -eq 0 ] && [ "$need_font" -eq 0 ]; then
    echo "System dependencies are up to date (mpv and fonts ready)."
    return 0
  fi

  echo ""
  echo "Checking system dependencies..."

  if [ "$OS" = "darwin" ]; then
    if [ "$need_mpv" -eq 1 ]; then
      if command -v brew >/dev/null 2>&1; then
        if prompt_consent "mpv is missing. Install via Homebrew? [y/N]"; then
          echo "Installing mpv with Homebrew..."
          brew install mpv || echo "Warning: Could not install mpv automatically. Run: brew install mpv"
        else
          echo "Skipped mpv installation."
          echo "   You can install it manually: brew install mpv"
          echo "   Note: Kickoff core score tracking works without mpv."
        fi
      else
        echo "Warning: Homebrew not found. Install mpv manually to enable video playback: https://mpv.io"
        echo "   Note: Kickoff core score tracking works without mpv."
      fi
    fi
    return 0
  fi

  # Linux Distribution Detection
  local distro="unknown"
  local distro_like=""
  if [ -f /etc/os-release ]; then
    distro="$(. /etc/os-release && echo "${ID:-unknown}")"
    distro_like="$(. /etc/os-release && echo "${ID_LIKE:-}")"
  elif [ -f /etc/debian_version ]; then
    distro="debian"
  elif [ -f /etc/arch-release ]; then
    distro="arch"
  elif [ -f /etc/fedora-release ]; then
    distro="fedora"
  fi

  local pkg_manager=""
  local font_pkg=""
  local mpv_pkg="mpv"

  case "$distro" in
    ubuntu|debian|pop|linuxmint|zorin|elementary)
      pkg_manager="apt"
      font_pkg="fonts-noto-color-emoji"
      ;;
    arch|manjaro|endeavouros|artix|cachyos)
      pkg_manager="pacman"
      font_pkg="noto-fonts-emoji"
      ;;
    fedora|rhel|centos|almalinux|rocky)
      pkg_manager="dnf"
      font_pkg="google-noto-color-emoji-fonts"
      ;;
    opensuse*|sles)
      pkg_manager="zypper"
      font_pkg="noto-coloremoji-fonts"
      ;;
    alpine)
      pkg_manager="apk"
      font_pkg="font-noto-emoji"
      ;;
    *)
      if echo "$distro_like" | grep -iq "debian"; then
        pkg_manager="apt"
        font_pkg="fonts-noto-color-emoji"
      elif echo "$distro_like" | grep -iq "arch"; then
        pkg_manager="pacman"
        font_pkg="noto-fonts-emoji"
      elif echo "$distro_like" | grep -iq "fedora\|rhel"; then
        pkg_manager="dnf"
        font_pkg="google-noto-color-emoji-fonts"
      elif echo "$distro_like" | grep -iq "suse"; then
        pkg_manager="zypper"
        font_pkg="noto-coloremoji-fonts"
      fi
      ;;
  esac

  local pkgs_to_install=()
  [ "$need_mpv" -eq 1 ] && pkgs_to_install+=("$mpv_pkg")
  [ "$need_font" -eq 1 ] && [ -n "$font_pkg" ] && pkgs_to_install+=("$font_pkg")

  if [ ${#pkgs_to_install[@]} -eq 0 ]; then
    return 0
  fi

  echo "Missing dependencies detected on ${distro}: ${pkgs_to_install[*]}"

  if ! prompt_consent "Install missing dependencies using sudo? [y/N]"; then
    echo "Skipping automatic dependency installation."
    print_manual_instructions "$pkg_manager" "${pkgs_to_install[@]}"
    return 0
  fi

  echo "Installing dependencies..."
  local success=0
  case "$pkg_manager" in
    apt)
      if run_elevated apt-get update -y && run_elevated apt-get install -y "${pkgs_to_install[@]}"; then
        success=1
      fi
      ;;
    pacman)
      if run_elevated pacman -Sy --noconfirm --needed "${pkgs_to_install[@]}"; then
        success=1
      fi
      ;;
    dnf)
      if run_elevated dnf install -y "${pkgs_to_install[@]}"; then
        success=1
      fi
      ;;
    zypper)
      if run_elevated zypper install -y "${pkgs_to_install[@]}"; then
        success=1
      fi
      ;;
    apk)
      if run_elevated apk add "${pkgs_to_install[@]}"; then
        success=1
      fi
      ;;
    *)
      echo "Warning: Package manager not recognized for ${distro}."
      ;;
  esac

  if [ "$success" -eq 1 ]; then
    echo "Dependencies installed successfully."
    if [ "$need_font" -eq 1 ] && command -v fc-cache >/dev/null 2>&1; then
      fc-cache -f >/dev/null 2>&1 || true
    fi
  else
    echo "Warning: Could not install dependencies automatically."
    print_manual_instructions "$pkg_manager" "${pkgs_to_install[@]}"
  fi
}

ensure_dependencies

if [ -n "$KICKOFF_SIGNALS_URL" ]; then
  echo ""
  echo "Importing configured signals pack..."
  if "$INSTALL_DIR/kickoff" --help 2>&1 | grep -q "config import"; then
    "$INSTALL_DIR/kickoff" config import "$KICKOFF_SIGNALS_URL" || true
  else
    echo "Note: This version of kickoff does not support 'config import'. Skipping signals import."
  fi
fi

echo ""
echo "All set! Run 'kickoff' to get started."
