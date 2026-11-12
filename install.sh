#!/usr/bin/env bash
# Installs the latest godoist release binary. You do not need Go.
# The INSTALL_DIR variable sets the target directory (default: ~/.local/bin).
set -Eeuo pipefail

REPO="biomassa/godoist"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"
TMP_DIR=""
OS=""
ARCH=""

log() {
    printf '[godoist] %s\n' "$*"
}

die() {
    printf '[godoist] ERROR: %s\n' "$*" >&2
    exit 1
}

cleanup() {
    if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
        rm -rf "$TMP_DIR"
    fi
}

fetch() {
    # fetch URL OUTPUT_FILE [curl options...]
    local url="$1" output="$2"
    shift 2

    curl -fsSL --proto '=https' --tlsv1.2 --retry 3 "$@" "$url" -o "$output"
}

require_commands() {
    local cmd

    for cmd in curl tar awk sed head find install mktemp tr uname; do
        command -v "$cmd" >/dev/null 2>&1 ||
            die "Missing dependency: $cmd"
    done

    if ! command -v sha256sum >/dev/null 2>&1 &&
       ! command -v shasum >/dev/null 2>&1; then
        die "sha256sum or shasum is required"
    fi
}

detect_platform() {
    case "$(uname -s)" in
        Linux) OS="linux" ;;
        Darwin) OS="darwin" ;;
        *) die "Unsupported operating system: $(uname -s)" ;;
    esac

    case "$(uname -m)" in
        x86_64 | amd64) ARCH="amd64" ;;
        aarch64 | arm64) ARCH="arm64" ;;
        *) die "Unsupported architecture: $(uname -m)" ;;
    esac

    log "Detected platform: ${OS}/${ARCH}"
}

get_release_tag() {
    sed -n \
        's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
        "$1" | head -n 1
}

# Prints the digest of one asset. Fails if there is not exactly one digest.
get_asset_digest() {
    local metadata="$1"
    local asset_name="$2"

    awk -v wanted="$asset_name" '
        /"assets":[[:space:]]*\[/ {
            in_assets = 1
        }

        in_assets && /"name":[[:space:]]*"/ {
            name = $0
            sub(/^.*"name":[[:space:]]*"/, "", name)
            sub(/".*$/, "", name)
            found = (name == wanted)
            next
        }

        in_assets && found && /"digest":[[:space:]]*"/ {
            digest = $0
            sub(/^.*"digest":[[:space:]]*"/, "", digest)
            sub(/".*$/, "", digest)
            count++
            print digest
            found = 0
        }

        END {
            if (count != 1) {
                exit 1
            }
        }
    ' "$metadata"
}

calculate_sha256() {
    local file="$1"

    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

verify_checksum() {
    local file="$1"
    local expected="$2"
    local actual

    [[ "$expected" =~ ^sha256:[a-fA-F0-9]{64}$ ]] ||
        die "Invalid SHA256 digest from GitHub: ${expected:-empty}"

    expected="${expected#sha256:}"
    actual="$(calculate_sha256 "$file")"

    expected="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"
    actual="$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')"

    [[ "$actual" == "$expected" ]] ||
        die "SHA256 verification failed"

    log "SHA256 verified successfully"
}

# Exits with status 0 if the installed binary has the wanted version.
check_current_version() {
    local wanted="$1"
    local current

    [[ -x "$INSTALL_DIR/godoist" ]] || return 0

    current="$("$INSTALL_DIR/godoist" --version </dev/null 2>/dev/null | awk '{print $NF}')" ||
        current=""

    if [[ "$current" == "$wanted" ]]; then
        log "godoist is already up-to-date (version $wanted)"
        exit 0
    fi

    log "Installed version: ${current:-unknown}"
}

main() {
    require_commands
    detect_platform

    TMP_DIR="$(mktemp -d)"
    trap cleanup EXIT

    log "Fetching latest release..."

    fetch "$API_URL" "$TMP_DIR/release.json" \
        -H "Accept: application/vnd.github+json" ||
        die "Could not fetch release data (network error or GitHub API rate limit)"

    local tag version
    tag="$(get_release_tag "$TMP_DIR/release.json")"

    [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
        die "Invalid release tag: ${tag:-empty}"

    version="${tag#v}"

    check_current_version "$version"

    local archive_name="godoist_${version}_${OS}_${ARCH}.tar.gz"

    log "Release: $tag"
    log "Asset: $archive_name"

    local expected_digest
    expected_digest="$(get_asset_digest "$TMP_DIR/release.json" "$archive_name")" ||
        die "Could not find exactly one SHA256 digest for $archive_name"

    log "Downloading archive..."

    fetch "https://github.com/${REPO}/releases/download/${tag}/${archive_name}" \
        "$TMP_DIR/$archive_name" ||
        die "Download failed"

    verify_checksum "$TMP_DIR/$archive_name" "$expected_digest"

    log "Extracting archive..."

    mkdir "$TMP_DIR/extracted"
    tar -xzf "$TMP_DIR/$archive_name" -C "$TMP_DIR/extracted"

    local binary
    binary="$(find "$TMP_DIR/extracted" -type f -name godoist -print -quit)"

    [[ -n "$binary" ]] ||
        die "godoist binary not found in archive"

    mkdir -p "$INSTALL_DIR"
    install -m 0755 "$binary" "$INSTALL_DIR/godoist"

    [[ -x "$INSTALL_DIR/godoist" ]] ||
        die "Installation failed"

    log "Installed: $INSTALL_DIR/godoist"

    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*)
            ;;
        *)
            log "Add this directory to your PATH:"
            printf 'export PATH="%s:$PATH"\n' "$INSTALL_DIR"
            ;;
    esac

    log "Installation completed: godoist $version"
}

main "$@"
