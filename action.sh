#!/bin/sh
# Install genguard and run genguard check for action.yml.
# Usage: action.sh install | action.sh check

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_dir/install.sh"

release_ref() {
  # github.action_ref is empty in a composite run step. action.yml copies it
  # into GENGUARD_ACTION_REF. GITHUB_ACTION_REF is not that value.
  ref=${GENGUARD_ACTION_REF:-}
  case "$ref" in
    refs/tags/*) ref=${ref#refs/tags/} ;;
  esac
  status=0
  valid_tag "$ref" || status=$?
  if [ "$status" -eq 0 ]; then
    printf '%s\n' "$ref"
    return 0
  fi
  # valid_tag status 1 is text outside the release shape. A v* ref then exits 2.
  if [ "$status" -eq 1 ]; then
    case "$ref" in
      v*)
        printf 'action.sh: %s is not a release tag (want v1.2.3)\n' "$ref" >&2
        return 2
        ;;
    esac
  fi
  return 1
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
