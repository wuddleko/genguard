#!/bin/sh
# Install genguard and run genguard check for action.yml.
# Usage: action.sh install | action.sh check

set -eu

release_ref() {
  # github.action_ref is empty in a composite run step. action.yml copies it
  # into GENGUARD_ACTION_REF. GITHUB_ACTION_REF is not that value.
  ref=${GENGUARD_ACTION_REF:-}
  case "$ref" in
    refs/tags/*) ref=${ref#refs/tags/} ;;
  esac
  case "$ref" in
    v[0-9]*.[0-9]*.[0-9]*|v[0-9]*.[0-9]*.[0-9]*-[A-Za-z0-9.]*) ;;
    v*)
      printf 'action.sh: %s is not a release tag (want v1.2.3)\n' "$ref" >&2
      return 2
      ;;
    *) return 1 ;;
  esac
  case "$ref" in
    *[!A-Za-z0-9.-]*) return 1 ;;
  esac
  printf '%s\n' "$ref"
}

install_from_source() {
  go -C "$GITHUB_ACTION_PATH" install -mod=mod ./cmd/genguard
  bindir=$(go env GOBIN)
  if [ -z "$bindir" ]; then
    bindir=$(go env GOPATH)/bin
  fi
  if [ -n "${GITHUB_PATH:-}" ]; then
    printf '%s\n' "$bindir" >> "$GITHUB_PATH"
  fi
}

install_release() {
  ref=$1
  base=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
  bindir=$base/genguard-bin
  mkdir -p "$bindir"
  PATH="$bindir${PATH:+:$PATH}" BINDIR="$bindir" sh "$GITHUB_ACTION_PATH/install.sh" "$ref"
  if [ -n "${GITHUB_PATH:-}" ]; then
    printf '%s\n' "$bindir" >> "$GITHUB_PATH"
  fi
}

install_genguard() {
  status=0
  ref=$(release_ref) || status=$?
  if [ "$status" -eq 0 ]; then
    install_release "$ref"
    return
  fi
  if [ "$status" -eq 2 ]; then
    exit 2
  fi
  install_from_source
}

run_check() {
  set -- check
  if [ "${GENGUARD_ALL:-}" = "true" ]; then
    set -- "$@" --all
  fi
  if [ -n "${GENGUARD_CONFIG:-}" ]; then
    set -- "$@" --config "$GENGUARD_CONFIG"
  fi
  if [ -n "${GENGUARD_SINCE:-}" ]; then
    set -- "$@" --since "$GENGUARD_SINCE"
  fi
  if [ "${GENGUARD_ISOLATED:-}" = "true" ]; then
    set -- "$@" --isolated
  fi
  genguard "$@"
}

case "${1:-}" in
  install) install_genguard ;;
  check) run_check ;;
  *)
    printf 'usage: action.sh install|check\n' >&2
    exit 2
    ;;
esac
