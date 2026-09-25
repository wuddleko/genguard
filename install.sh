#!/bin/sh
# Download a genguard release and install the binary.
# Usage: install.sh [tag]

set -eu

default_tag=v0.5.0
repo=wuddleko/genguard

die() {
  printf 'install.sh: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<EOF
usage: install.sh [tag]

Downloads a genguard release and installs the binary.
The default tag is ${default_tag}. A tag argument, or GENGUARD_TAG, overrides it.

BINDIR sets the install directory. It has to already be on PATH.
When BINDIR is unset, the script picks the first of these that is on PATH:
  /usr/local/bin  if that directory is writable, or if sudo can run without a password
  /opt/homebrew/bin  if that directory is writable
  \$HOME/.local/bin
If none of those is on PATH, the script exits with an error.

GENGUARD_OS and GENGUARD_ARCH override OS and architecture detection.
EOF
}

# Status: 0 release tag, 1 other text, 2 release shape with a disallowed character.
valid_tag() {
  case "$1" in
    v[0-9]*.[0-9]*.[0-9]*|v[0-9]*.[0-9]*.[0-9]*-[A-Za-z0-9.]*) ;;
    *) return 1 ;;
  esac
  case "$1" in
    *[!A-Za-z0-9.-]*) return 2 ;;
  esac
  return 0
}

on_path() {
  dir=$1
  rest=$PATH
  while [ -n "$rest" ]; do
    case "$rest" in
      *:*) entry=${rest%%:*}; rest=${rest#*:} ;;
      *) entry=$rest; rest= ;;
    esac
    if [ "$entry" = "$dir" ]; then
      return 0
    fi
  done
  return 1
}

resolve_platform() {
  os=${GENGUARD_OS:-}
  if [ -z "$os" ]; then
    case "$(uname -s)" in
      Linux) os=linux ;;
      Darwin) os=darwin ;;
      MINGW*|MSYS*|CYGWIN*|Windows_NT) os=windows ;;
      *) die "unsupported OS: $(uname -s)" ;;
    esac
  fi

  arch=${GENGUARD_ARCH:-}
  if [ -z "$arch" ]; then
    case "$(uname -m)" in
      x86_64|amd64) arch=amd64 ;;
      arm64|aarch64) arch=arm64 ;;
      *) die "unsupported architecture: $(uname -m)" ;;
    esac
  fi

  case "$os/$arch" in
    linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
    *) die "no release for ${os}/${arch}" ;;
  esac
}

resolve_asset() {
  version=${tag#v}
  case "$os" in
    windows)
      asset="genguard_${version}_${os}_${arch}.zip"
      binary=genguard.exe
      ;;
    *)
      asset="genguard_${version}_${os}_${arch}.tar.gz"
      binary=genguard
      ;;
  esac
}

checksum_hash() {
  awk -v name="$asset" '
    $NF == name { print $1; found = 1; exit }
    END { if (!found) exit 1 }
  ' "$1"
}

verify_archive() {
  sumfile=$1
  command -v awk >/dev/null 2>&1 || die "awk is required to read checksums.txt"
  hash=$(checksum_hash "$sumfile") || die "checksums.txt has no entry for ${asset}"
  printf '%s  %s\n' "$hash" "$asset" > "$tmpdir/check.txt"
  (
    cd "$tmpdir"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c check.txt
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 -c check.txt
    else
      die "sha256sum or shasum is required to verify the release"
    fi
  )
}

extract_binary() {
  if [ "$os" = windows ]; then
    if command -v unzip >/dev/null 2>&1; then
      unzip -oj "$tmpdir/$asset" "$binary" -d "$tmpdir"
      return
    fi
    tar -xf "$tmpdir/$asset" -C "$tmpdir" "$binary"
    return
  fi
  tar -xzf "$tmpdir/$asset" -C "$tmpdir" "$binary"
}

# GENGUARD_INSTALL_ROOT prefixes the default bindirs. Tests set it.
choose_bindir() {
  root=${GENGUARD_INSTALL_ROOT:-}
  sys_bindir=${root}/usr/local/bin
  brew_bindir=${root}/opt/homebrew/bin
  use_sudo=0
  if [ -n "${BINDIR:-}" ]; then
    bindir=$BINDIR
  elif on_path "$sys_bindir" && [ -d "$sys_bindir" ] && [ -w "$sys_bindir" ]; then
    bindir=$sys_bindir
  elif on_path "$sys_bindir" && command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    bindir=$sys_bindir
    use_sudo=1
  elif on_path "$brew_bindir" && [ -d "$brew_bindir" ] && [ -w "$brew_bindir" ]; then
    bindir=$brew_bindir
  elif on_path "${HOME:?}/.local/bin"; then
    bindir=${HOME}/.local/bin
  else
    die "no writable directory on PATH; set BINDIR to one"
  fi
  on_path "$bindir" || die "${bindir} is not on PATH"
  if [ "$use_sudo" = 1 ]; then
    sudo -n mkdir -p "$bindir"
  else
    mkdir -p "$bindir"
  fi
}

install_binary() {
  src=$tmpdir/$binary
  dest=$bindir/$binary
  if [ ! -f "$src" ]; then
    die "archive did not contain ${binary}"
  fi
  on_path "$bindir" || die "${bindir} is not on PATH"
  if [ "$use_sudo" = 1 ]; then
    sudo -n install -m 0755 "$src" "$dest"
  elif command -v install >/dev/null 2>&1; then
    install -m 0755 "$src" "$dest"
  else
    cp "$src" "$dest"
    chmod 0755 "$dest"
  fi
  printf 'installed %s\n' "$dest"
}

# action.sh sources this file for valid_tag. A downloaded copy may not be named install.sh.
case "${0##*/}" in
  action.sh) return 0 ;;
esac

tag=${GENGUARD_TAG:-$default_tag}
print_asset=0
print_bindir=0

for arg in "$@"; do
  case "$arg" in
    --print-asset) print_asset=1 ;;
    --print-bindir) print_bindir=1 ;;
    -h|--help) usage; exit 0 ;;
    --) ;;
    -*) die "unknown option: $arg" ;;
    *) tag=$arg ;;
  esac
done

valid_tag "$tag" || die "tag must look like v1.2.3 (got ${tag})"
resolve_platform

if [ "$print_bindir" = 1 ]; then
  choose_bindir
  printf '%s\n' "$bindir"
  exit 0
fi

resolve_asset

if [ "$print_asset" = 1 ]; then
  printf '%s\n' "$asset"
  exit 0
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

base="https://github.com/${repo}/releases/download/${tag}"
curl -fsSL "$base/$asset" -o "$tmpdir/$asset" || die "download failed: $base/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmpdir/checksums.txt" || die "download failed: $base/checksums.txt"
verify_archive "$tmpdir/checksums.txt"
extract_binary
choose_bindir
install_binary
