#!/usr/bin/env sh
set -e

REPO="NirajNair/syncdoc"
BINARY="syncdoc"

# ── Helpers ──────────────────────────────────────────────────────────

info()  { printf "[INFO]  %s\\n" "$1"; }
warn()  { printf "[WARN]  %s\\n" "$1" >&2; }
error() { printf "[ERROR] %s\\n" "$1" >&2; }

cleanup() {
    if [ -n "$TMPDIR" ] && [ -d "$TMPDIR" ]; then
        rm -rf "$TMPDIR"
    fi
}
trap cleanup EXIT

# ── Detect platform ──────────────────────────────────────────────────
# goreleaser name_template uses title(Os) so we need Darwin / Linux / Windows
# and maps amd64 → x86_64 in the archive name.

get_os() {
    case "$(uname -s)" in
        Darwin*)  echo "Darwin" ;;
        Linux*)   echo "Linux" ;;
        MINGW*|MSYS*|CYGWIN*) echo "Windows" ;;
        *)        error "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac
}

get_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)            error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac
}

get_extension() {
    case "$(get_os)" in
        Windows) echo "zip" ;;
        *)       echo "tar.gz" ;;
    esac
}

# ── Get latest version from GitHub ───────────────────────────────────

get_latest_version() {
    _version=$(curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name":' \
        | head -n1 \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

    if [ -z "$_version" ]; then
        error "Could not determine latest version from GitHub API."
        error "Check your internet connection and that https://github.com/${REPO}/releases is accessible."
        exit 1
    fi
    echo "$_version"
}

# ── Verify a binary is executable on this system ──────────────────────

check_prerequisites() {
    for cmd in curl; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            error "Required command '${cmd}' not found. Please install it and try again."
            exit 1
        fi
    done

    OS=$(get_os)
    if [ "$OS" = "Windows" ]; then
        if ! command -v unzip >/dev/null 2>&1; then
            error "Required command 'unzip' not found. Please install it and try again."
            exit 1
        fi
    else
        if ! command -v tar >/dev/null 2>&1; then
            error "Required command 'tar' not found. Please install it and try again."
            exit 1
        fi
    fi
}

# ── Main ─────────────────────────────────────────────────────────────

INSTALL_DIR="/usr/local/bin"

check_prerequisites

info "Installing ${BINARY}..."

VERSION=$(get_latest_version)

OS=$(get_os)
ARCH=$(get_arch)
EXT=$(get_extension)

# Archive naming must match goreleaser name_template:
#   {{ .ProjectName }}_{{ title .Os }}_{{ if eq .Arch "amd64" }}x86_64{{ else }}arm64{{ end }}
# No version in the archive name.
ARCHIVE_NAME="${BINARY}_${OS}_${ARCH}.${EXT}"

URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"

info "Downloading ${BINARY} ${VERSION} for ${OS}/${ARCH}..."
info "  URL: ${URL}"

TMPDIR=$(mktemp -d "${TMPDIR:-/tmp}/${BINARY}-install.XXXXXX")

HTTP_CODE=$(curl -sfL -w "%{http_code}" -o "${TMPDIR}/${ARCHIVE_NAME}" "$URL" 2>/dev/null) || true

if [ "$HTTP_CODE" != "200" ]; then
    error "Download failed (HTTP ${HTTP_CODE:-unknown})."
    error "  URL: ${URL}"
    error "  This may mean the release asset '${ARCHIVE_NAME}' does not exist yet."
    error "  Check available assets at: https://github.com/${REPO}/releases/${VERSION}"
    exit 1
fi

FILE_SIZE=$(wc -c < "${TMPDIR}/${ARCHIVE_NAME}" 2>/dev/null | tr -d ' ')
if [ "$FILE_SIZE" -lt 100 ] 2>/dev/null; then
    error "Downloaded file is suspiciously small (${FILE_SIZE} bytes). The download may have failed."
    error "  Expected a release archive, got: ${TMPDIR}/${ARCHIVE_NAME}"
    head -c 200 "${TMPDIR}/${ARCHIVE_NAME}" >&2
    echo "" >&2
    exit 1
fi

info "Extracting..."
mkdir -p "${TMPDIR}/out"

if [ "$EXT" = "zip" ]; then
    if ! unzip -o "${TMPDIR}/${ARCHIVE_NAME}" -d "${TMPDIR}/out"; then
        error "Failed to extract zip archive."
        exit 1
    fi
else
    if ! tar -xzf "${TMPDIR}/${ARCHIVE_NAME}" -C "${TMPDIR}/out"; then
        error "Failed to extract tar.gz archive."
        exit 1
    fi
fi

BINARY_PATH="${TMPDIR}/out/${BINARY}"
if [ ! -f "$BINARY_PATH" ]; then
    # Search for binary inside subdirectories (goreleaser may nest it)
    BINARY_PATH=$(find "${TMPDIR}/out" -name "${BINARY}" -type f 2>/dev/null | head -n1)
    if [ -z "$BINARY_PATH" ]; then
        error "Binary '${BINARY}' not found in archive after extraction."
        error "Archive contents:"
        ls -lR "${TMPDIR}/out" >&2
        exit 1
    fi
fi

if [ -w "$INSTALL_DIR" ]; then
    cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
else
    info "sudo required to install to ${INSTALL_DIR}"
    sudo cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

info "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"
info "Run '${BINARY} --help' to get started."