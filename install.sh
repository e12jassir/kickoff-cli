#!/usr/bin/env bash
set -e

# kickoff-cli universal installer for Linux and macOS
REPO="e12jassir/kickoff-cli"
VERSION="0.1.2"
INSTALL_DIR="$HOME/.local/bin"

mkdir -p "$INSTALL_DIR"

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
    echo "❌ Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux)
    ;;
  darwin)
    OS="macos"
    ;;
  *)
    echo "❌ Unsupported operating system: $OS. For Windows, download kickoff.exe from releases."
    exit 1
    ;;
esac

echo "⚽ Installing kickoff-cli v${VERSION} for ${OS}/${ARCH}..."

TARBALL_URL="https://github.com/${REPO}/releases/download/v${VERSION}/kickoff_${VERSION}_${OS}_${ARCH}.tar.gz"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

DOWNLOADED=0
if curl -fsSL -o "$TEMP_DIR/kickoff.tar.gz" "$TARBALL_URL" 2>/dev/null; then
  tar -xzf "$TEMP_DIR/kickoff.tar.gz" -C "$TEMP_DIR"
  if [ -f "$TEMP_DIR/kickoff" ]; then
    install -m 755 "$TEMP_DIR/kickoff" "$INSTALL_DIR/kickoff"
    DOWNLOADED=1
  fi
fi

if [ "$DOWNLOADED" -eq 0 ]; then
  echo "ℹ️  Release asset not found yet. Building from source via Git..."
  if ! command -v go >/dev/null 2>&1; then
    echo "❌ Error: Go 1.24+ is required to build from source when precompiled assets are not found."
    echo "   Install Go or download the release package from: https://github.com/${REPO}/releases"
    exit 1
  fi
  git clone --depth 1 "https://github.com/${REPO}.git" "$TEMP_DIR/repo"
  (cd "$TEMP_DIR/repo" && go build -ldflags="-s -w" -o "$INSTALL_DIR/kickoff" ./cmd/kickoff)
fi

echo "✅ kickoff instalado exitosamente en: $INSTALL_DIR/kickoff"

# Verify PATH
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
  echo "⚠️  Nota: Asegúrate de tener $INSTALL_DIR en tu PATH:"
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

has_emoji_font() {
  if [ "$OS" = "darwin" ] || [ "$OS" = "macos" ]; then
    return 0 # macOS has native Apple Color Emoji
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
    echo "✨ Dependencias del sistema al día (mpv y fuentes emoji listas)."
    return 0
  fi

  echo ""
  echo "🔍 Detectando dependencias del sistema requeridas..."

  if [ "$OS" = "darwin" ] || [ "$OS" = "macos" ]; then
    if [ "$need_mpv" -eq 1 ]; then
      if command -v brew >/dev/null 2>&1; then
        echo "📦 Instalando mpv con Homebrew..."
        brew install mpv || echo "⚠️  No se pudo instalar mpv automáticamente. Ejecuta: brew install mpv"
      else
        echo "⚠️  Homebrew no detectado. Instala mpv manualmente para reproducir señales: https://mpv.io"
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

  echo "📦 Dependencias faltantes detectadas en ${distro}: ${pkgs_to_install[*]}"
  echo "🚀 Instalando dependencias automáticamente..."

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
      echo "⚠️  Gestor de paquetes no reconocido para ${distro}."
      ;;
  esac

  if [ "$success" -eq 1 ]; then
    echo "✅ Dependencias instaladas correctamente."
    if [ "$need_font" -eq 1 ]; then
      fc-cache -f >/dev/null 2>&1 || true
    fi
  else
    echo "⚠️  No se pudieron instalar automáticamente las dependencias (se requieren permisos de administrador)."
    echo "   Puedes instalarlas manualmente según tu distribución:"
    case "$pkg_manager" in
      apt)    echo "   sudo apt install ${pkgs_to_install[*]}" ;;
      pacman) echo "   sudo pacman -S ${pkgs_to_install[*]}" ;;
      dnf)    echo "   sudo dnf install ${pkgs_to_install[*]}" ;;
      zypper) echo "   sudo zypper install ${pkgs_to_install[*]}" ;;
      *)      echo "   Instala: ${pkgs_to_install[*]}" ;;
    esac
  fi
}

ensure_dependencies

echo ""
echo "⚽ ¡Listo! Ejecuta 'kickoff' para iniciar."
