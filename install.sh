#!/usr/bin/env bash
set -euo pipefail

# avellcc installer
# Usage: curl -fsSL https://raw.githubusercontent.com/hugo-andrade/avellcc/main/install.sh | bash
#
# Environment variables:
#   INSTALL_DIR  - Binary install directory (default: /usr/local/bin)
#   VERSION      - Specific version to install (default: latest)

REPO="hugo-andrade/avellcc"

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# --- Colors ---
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf "${CYAN}%s${RESET}\n" "$*"; }
ok()    { printf "${GREEN}%s${RESET}\n" "$*"; }
warn()  { printf "${YELLOW}warning: %s${RESET}\n" "$*" >&2; }
err()   { printf "${RED}error: %s${RESET}\n" "$*" >&2; exit 1; }

# --- Detect platform ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       err "unsupported architecture: $ARCH" ;;
esac

[ "$OS" = "linux" ] || err "avellcc only supports Linux (got: $OS)"

# --- Check dependencies ---
command -v curl >/dev/null 2>&1 || err "curl is required but not installed"
command -v tar  >/dev/null 2>&1 || err "tar is required but not installed"

SHA256_TOOL=""
if command -v sha256sum >/dev/null 2>&1; then
    SHA256_TOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA256_TOOL="shasum"
else
    warn "no checksum tool found (sha256sum/shasum); skipping integrity verification"
fi

# --- Get version ---
if [ -z "${VERSION:-}" ]; then
    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | cut -d'"' -f4)
    [ -n "$VERSION" ] || err "could not determine latest version"
fi
if [ "${VERSION#v}" = "${VERSION}" ]; then
    VERSION="v${VERSION}"
fi

info "Installing avellcc ${VERSION} (${OS}/${ARCH})..."

# --- Download and verify ---
TARBALL="avellcc-${VERSION#v}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"
CHECKSUMS_FILE="avellcc-${VERSION#v}-checksums.txt"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/${CHECKSUMS_FILE}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading ${TARBALL}..."
curl -fsSL "$URL" -o "${TMP}/${TARBALL}" || err "download failed — check that version ${VERSION} exists"

if [ -n "$SHA256_TOOL" ]; then
    if curl -fsSL "${CHECKSUMS_URL}" -o "${TMP}/${CHECKSUMS_FILE}" 2>/dev/null; then
        TARGET_LINE=$(grep "${TARBALL}" "${TMP}/${CHECKSUMS_FILE}" || true)
        if [ -n "$TARGET_LINE" ]; then
            echo "$TARGET_LINE" > "${TMP}/check.txt"
            info "Verifying checksum..."
            if [ "$SHA256_TOOL" = "sha256sum" ]; then
                (cd "$TMP" && sha256sum -c check.txt) || err "checksum verification failed"
            else
                (cd "$TMP" && shasum -a 256 -c check.txt) || err "checksum verification failed"
            fi
            ok "Checksum verified"
        else
            warn "no checksum entry for ${TARBALL}; skipping verification"
        fi
    else
        warn "checksum file not found; skipping verification"
    fi
fi

# --- Extract and install ---
tar -xzf "${TMP}/${TARBALL}" -C "$TMP" || err "extraction failed"

mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
    install -m755 "${TMP}/avellcc" "${INSTALL_DIR}/avellcc"
else
    sudo install -m755 "${TMP}/avellcc" "${INSTALL_DIR}/avellcc"
fi
ok "Installed avellcc to ${INSTALL_DIR}/avellcc"

# --- Install udev rules ---
UDEV_SRC="${TMP}/udev/99-avell.rules"
UDEV_DIR="/etc/udev/rules.d"

if [ -f "$UDEV_SRC" ]; then
    info "Installing udev rules..."
    sudo cp "$UDEV_SRC" "$UDEV_DIR/"
    sudo udevadm control --reload-rules && sudo udevadm trigger
    ok "udev rules installed (non-root access to keyboard and lightbar)"
else
    echo ""
    printf "${BOLD}NOTE:${RESET} udev rules were not included in this release.\n"
    printf "For non-root access, create %s/99-avell.rules with:\n" "$UDEV_DIR"
    printf '  SUBSYSTEM=="hidraw", ATTRS{idVendor}=="048d", ATTRS{idProduct}=="8910", MODE="0666"\n'
    printf '  SUBSYSTEM=="hidraw", ATTRS{idVendor}=="048d", ATTRS{idProduct}=="8911", MODE="0666"\n'
fi

# --- Install systemd service ---
SYSTEMD_SRC="${TMP}/systemd/avellcc-restore.service"
SYSTEMD_DIR="/etc/systemd/system"

if [ -f "$SYSTEMD_SRC" ]; then
    echo ""
    info "Installing systemd service..."
    sudo cp "$SYSTEMD_SRC" "$SYSTEMD_DIR/"
    sudo systemctl daemon-reload
    ok "systemd service installed"
    printf "  Enable restore on boot: ${CYAN}sudo systemctl enable avellcc-restore.service${RESET}\n"
fi

# --- Verify PATH ---
echo ""
if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "avellcc installed successfully"
    printf "  Run: ${CYAN}avellcc --help${RESET}\n"
else
    ok "avellcc installed successfully"
    printf "${BOLD}NOTE:${RESET} %s is not in your PATH.\n" "$INSTALL_DIR"
    printf "  Add it: export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
fi
