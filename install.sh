#!/usr/bin/env sh
set -e

REPO="NirajNair/syncdoc"
BINARY="syncdoc"

get_os() {
    case "$(uname -s)" in
        Darwin*) echo "darwin" ;;
        Linux*) echo "linux" ;;
        *) echo "Error: Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
}

get_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "Error: Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
}

get_latest_version() {
    curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" |
        grep '"tag_name":' |
        sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
}

get_extension() {
    os=$(get_os)
    case "$os" in
        windows) echo "zip" ;;
        *) echo "tar.gz" ;;
    esac
}

INSTALL_DIR="/usr/local/bin"

echo "Installing ${BINARY}..."

VERSION=$(get_latest_version)
if [ -z "$VERSION" ]; then
    echo "Error: Could not determine latest version" >&2
    exit 1
fi

OS=$(get_os)
ARCH=$(get_arch)
EXT=$(get_extension)
ARCHIVE_NAME="${BINARY}_${VERSION#"v"}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"

echo "Downloading ${BINARY} ${VERSION} for ${OS}/${ARCH}..."
TMPDIR=$(mktemp -d)
curl -sfL "$URL" -o "${TMPDIR}/${ARCHIVE_NAME}"

echo "Extracting..."
if [ "$EXT" = "zip" ]; then
    unzip -o "${TMPDIR}/${ARCHIVE_NAME}" -d "${TMPDIR}/out"
else
    tar -xzf "${TMPDIR}/${ARCHIVE_NAME}" -C "${TMPDIR}/out"
fi

BINARY_PATH="${TMPDIR}/out/${BINARY}"
if [ ! -f "$BINARY_PATH" ]; then
    echo "Error: Binary not found in archive" >&2
    rm -rf "$TMPDIR"
    exit 1
fi

if [ -w "$INSTALL_DIR" ]; then
    cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
else
    echo "sudo required to install to ${INSTALL_DIR}"
    sudo cp "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

rm -rf "$TMPDIR"

echo "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"
echo "Run '${BINARY} --help' to get started."