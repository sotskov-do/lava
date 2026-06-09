#!/usr/bin/env bash
# Resolves create_spec.yml inputs from either a manual workflow_dispatch or a
# repository_dispatch sent by the Jira Automation rule. Reads env vars (never
# interpolates untrusted payload into shell code) and writes key=value lines to
# $GITHUB_OUTPUT (stdout when unset, for local testing). Exits non-zero on a
# malformed repository_dispatch title, after emitting issue_key so the workflow
# callback can still label the ticket spec-failed.
set -euo pipefail

out="${GITHUB_OUTPUT:-/dev/stdout}"

# Always use the heredoc output form so multiline values (e.g. a ticket
# description) are emitted safely.
emit() {
  {
    printf '%s<<__GHA_EOF__\n' "$1"
    printf '%s\n' "$2"
    printf '__GHA_EOF__\n'
  } >> "$out"
}

trim() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"   # leading
  s="${s%"${s##*[![:space:]]}"}"   # trailing
  printf '%s' "$s"
}

case "${EVENT_NAME:?EVENT_NAME required}" in
  workflow_dispatch)
    emit chain_name          "${WD_CHAIN_NAME:-}"
    emit chain_mainnet_index "${WD_MAINNET:-}"
    emit chain_testnet_index "${WD_TESTNET:-}"
    emit additional_data     "${WD_ADDITIONAL:-}"
    emit issue_key           ""
    ;;
  repository_dispatch)
    title="${RD_TITLE:-}"
    # Emit issue_key FIRST so a parse failure still lets the callback report.
    emit issue_key "${RD_ISSUE_KEY:-}"
    case "$title" in
      "[create-spec]"*) ;;
      *) echo "Title must start with '[create-spec]': '$title'" >&2; exit 1 ;;
    esac
    rest="${title#*]}"                 # drop the [create-spec] prefix
    IFS='/' read -r f1 f2 f3 extra <<<"$rest"
    chain_name="$(trim "${f1:-}")"
    mainnet="$(trim "${f2:-}")"
    testnet="$(trim "${f3:-}")"
    if [ -z "$chain_name" ] || [ -z "$mainnet" ] || [ -z "$testnet" ] || [ -n "${extra:-}" ]; then
      echo "Title must be '[create-spec] Name / MAINNET / TESTNET', got: '$title'" >&2
      exit 1
    fi
    emit chain_name          "$chain_name"
    emit chain_mainnet_index "$mainnet"
    emit chain_testnet_index "$testnet"
    emit additional_data     "${RD_DESCRIPTION:-}"
    ;;
  *)
    echo "Unsupported event: $EVENT_NAME" >&2; exit 1 ;;
esac
