#!/usr/bin/env bash
# check_verifications.sh — verify schema + expected_value for every verifications[] entry.
set -euo pipefail
export LC_ALL=C

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <spec.json>" >&2
  exit 2
fi
SPEC=$(realpath -- "$1")
[[ -r "$SPEC" ]] || { echo "cannot read spec: $SPEC" >&2; exit 1; }

PASS=()
FAIL=()
INFO=()

# Extract: idx | collection_iface | ver_name | has_parse_directive | pd_function_tag | num_values | first_expected_value | severity
while IFS=$'\t' read -r idx iface name has_pd pd_tag nvals val0 sev; do
  [[ -z "$idx" ]] && continue
  ROW="$idx/$iface/$name"

  # Required fields
  if [[ "$name" == "null" || -z "$name" ]]; then
    FAIL+=("$ROW|missing name"); continue
  fi
  if [[ "$has_pd" != "true" ]]; then
    FAIL+=("$ROW|missing parse_directive")
  fi
  if [[ "$nvals" == "0" || "$nvals" == "null" ]]; then
    FAIL+=("$ROW|missing values[]")
  else
    # expected_value check
    if [[ -z "$val0" || "$val0" == "null" ]]; then
      FAIL+=("$ROW|values[0].expected_value missing")
    elif [[ "$val0" == "*" ]]; then
      INFO+=("$ROW|values[0].expected_value is wildcard '*'")
    else
      PASS+=("$ROW|expected_value=$val0")
    fi
  fi

  # severity enum
  case "$sev" in
    Warning|Fail|Stop) PASS+=("$ROW|severity=$sev") ;;
    null|"") FAIL+=("$ROW|severity missing") ;;
    *) FAIL+=("$ROW|severity invalid ($sev), expected Warning|Fail|Stop") ;;
  esac

done < <(jq -r '
  .proposal.specs[] as $s
  | $s.api_collections[]? as $c
  | $c.verifications[]?
  | "\($s.index)\t\($c.collection_data.api_interface)\t\(.name // "null")\t\(.parse_directive != null)\t\(.parse_directive.function_tag // "null")\t\(.values | length)\t\(if (.values[0].expected_value? // null) == null or (.values[0].expected_value? // null) == "" then "null" else .values[0].expected_value end)\t\(.severity // "null")"
' "$SPEC")

# Cross-reference: every verification's parse_directive.function_tag must exist in its same collection's parse_directives[]
while IFS=$'\t' read -r idx iface name pd_tag; do
  [[ -z "$idx" || "$pd_tag" == "null" ]] && continue
  ROW="$idx/$iface/$name"
  EXISTS=$(jq -r --arg idx "$idx" --arg iface "$iface" --arg tag "$pd_tag" '
    .proposal.specs[] | select(.index == $idx)
    | .api_collections[]
    | select(.collection_data.api_interface == $iface)
    | [.parse_directives[]?.function_tag] | contains([$tag])
  ' "$SPEC")
  if [[ "$EXISTS" == "true" ]]; then
    PASS+=("$ROW|parse_directive ref ($pd_tag) found in collection")
  else
    FAIL+=("$ROW|parse_directive ref ($pd_tag) NOT found in collection")
  fi
done < <(jq -r '
  .proposal.specs[] as $s
  | $s.api_collections[]? as $c
  | $c.verifications[]?
  | select(.parse_directive != null)
  | "\($s.index)\t\($c.collection_data.api_interface)\t\(.name)\t\(.parse_directive.function_tag)"
' "$SPEC")

echo "=== PASS ==="
printf '%s\n' ${PASS[@]+"${PASS[@]}"}
echo
echo "=== FAIL ==="
printf '%s\n' ${FAIL[@]+"${FAIL[@]}"}
echo
echo "=== INFO ==="
printf '%s\n' ${INFO[@]+"${INFO[@]}"}

[[ ${#FAIL[@]} -eq 0 ]] || exit 1
