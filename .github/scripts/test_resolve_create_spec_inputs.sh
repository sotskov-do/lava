#!/usr/bin/env bash
# Tests for resolve_create_spec_inputs.sh. Runs the resolver with a given env,
# captures its $GITHUB_OUTPUT, and asserts on the emitted key=value lines.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$HERE/resolve_create_spec_inputs.sh"
fails=0

# run <expected_exit> <env assignments...> -- captures outputs into $OUT_FILE
run() {
  local want_exit="$1"; shift
  OUT_FILE="$(mktemp)"
  env -i GITHUB_OUTPUT="$OUT_FILE" "$@" bash "$SCRIPT" >/dev/null 2>"$OUT_FILE.err"
  GOT_EXIT="$?"
  if [ "$GOT_EXIT" != "$want_exit" ]; then
    echo "FAIL: expected exit $want_exit, got $GOT_EXIT (env: $*)"
    echo "  stderr: $(cat "$OUT_FILE.err")"
    fails=$((fails+1))
  fi
}

# assert that $OUT_FILE contains a single-line output key=value
assert_out() {
  local key="$1" val="$2"
  # outputs use heredoc form: key<<EOF \n value \n EOF
  if ! awk -v k="$key" -v v="$val" '
      $0==k"<<__GHA_EOF__"{g=1; next}
      g==1{ if($0==v) found=1; g=2 }
      END{ exit(found?0:1) }' "$OUT_FILE"; then
    echo "FAIL: expected output $key=$val"
    echo "  got: $(cat "$OUT_FILE")"
    fails=$((fails+1))
  fi
}

# 1. repository_dispatch happy path
run 0 EVENT_NAME=repository_dispatch \
      RD_TITLE='[create-spec] Iota / IOTA / IOTAT' \
      RD_DESCRIPTION='some docs' RD_ISSUE_KEY=LAVA-42
assert_out chain_name Iota
assert_out chain_mainnet_index IOTA
assert_out chain_testnet_index IOTAT
assert_out additional_data 'some docs'
assert_out issue_key LAVA-42

# 2. extra whitespace is trimmed
run 0 EVENT_NAME=repository_dispatch \
      RD_TITLE='[create-spec]    Foo Bar  /  FOO /  FOOT  ' \
      RD_DESCRIPTION='' RD_ISSUE_KEY=LAVA-1
assert_out chain_name 'Foo Bar'
assert_out chain_mainnet_index FOO
assert_out chain_testnet_index FOOT

# 3. missing a slash -> exit 1, but issue_key still emitted
run 1 EVENT_NAME=repository_dispatch \
      RD_TITLE='[create-spec] Iota / IOTA' \
      RD_DESCRIPTION='' RD_ISSUE_KEY=LAVA-7
assert_out issue_key LAVA-7

# 4. missing prefix -> exit 1
run 1 EVENT_NAME=repository_dispatch \
      RD_TITLE='Iota / IOTA / IOTAT' \
      RD_DESCRIPTION='' RD_ISSUE_KEY=LAVA-8
assert_out issue_key LAVA-8

# 5. workflow_dispatch passthrough, empty issue_key
run 0 EVENT_NAME=workflow_dispatch \
      WD_CHAIN_NAME=Iota WD_MAINNET=IOTA WD_TESTNET=IOTAT \
      WD_ADDITIONAL='hint'
assert_out chain_name Iota
assert_out chain_mainnet_index IOTA
assert_out chain_testnet_index IOTAT
assert_out additional_data hint
assert_out issue_key ''

# 6. multiline description survives (heredoc output form)
run 0 EVENT_NAME=repository_dispatch \
      RD_TITLE='[create-spec] Iota / IOTA / IOTAT' \
      RD_DESCRIPTION=$'line1\nline2' RD_ISSUE_KEY=LAVA-9
assert_out chain_name Iota

if [ "$fails" -eq 0 ]; then echo "ALL PASS"; else echo "$fails FAILED"; exit 1; fi
