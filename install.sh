#!/usr/bin/env bash
set -e

# kickoff-cli universal installer for Linux and macOS
REPO="e12jassir/kickoff-cli"
VERSION="0.1.0"
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
  linux|darwin)
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

# Check mpv
if ! command -v mpv >/dev/null 2>&1; then
  echo ""
  echo "⚠️  Recordatorio: Recuerda instalar mpv para reproducir partidos:"
  if [ "$OS" = "darwin" ]; then
    echo "   brew install mpv"
  elif command -v pacman >/dev/null 2>&1; then
    echo "   sudo pacman -S mpv"
  elif command -v apt-get >/dev/null 2>&1; then
    echo "   sudo apt install mpv"
  elif command -v dnf >/dev/null 2>&1; then
    echo "   sudo dnf install mpv"
  fi
fi

echo ""
echo "⚽ ¡Listo! Ejecuta 'kickoff' para iniciar."
