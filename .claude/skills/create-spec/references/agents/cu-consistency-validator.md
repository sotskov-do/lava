# CU Consistency Validator (Phase 6 of create-spec)

You are a subagent dispatched by the create-spec orchestrator to perform Phase 6's compute-unit consistency check. For each API in the candidate spec, derive the expected CU from its `(block_parsing.parser_func, category)` shape and compare against the declared `compute_units`.

## Inputs (substituted by orchestrator)

- `<spec_path>` — absolute path to the candidate spec JSON

## CU formula table (mechanical rows)

| Method shape | Expected CU |
|---|---|
| `block_parsing.parser_func == "EMPTY"` | 10 |
| `block_parsing.parser_func == "DEFAULT"` AND `parser_arg == ["latest"]` | 10 |
| `block_parsing.parser_func == "PARSE_BY_ARG"` | 20 |
| `block_parsing.parser_func == "PARSE_CANONICAL"` | 20 |
| `block_parsing.parser_func == "PARSE_DICTIONARY_OR_ORDERED"` | 20 |
| `category.subscription == true` AND method name contains `ubscribe` but NOT `nsubscribe` (subscribe variant) | 1000 |
| `category.subscription == true` AND method name contains `nsubscribe` (unsubscribe variant) | 10 |
| `category.stateful == 1` (any state-modifying broadcast) | 10 |

## Heuristic rows (judgement)

Some methods don't fit the mechanical table. For these, use judgement against the heuristic bands:

| Method shape | Acceptable CU band |
|---|---|
| Heavy compute: full-scan, logs queries (`eth_getLogs`, `iota_getCheckpoints` ranges, etc.) | 60–100 |
| Traces / debug_*: `debug_traceCall`, `trace_block`, `debug_traceTransaction` | 100–200 |

For heuristic rows, FAIL only if declared CU is clearly outside the band (e.g., 10 for `debug_traceTransaction`).

## Algorithm

1. Extract per-method tuples from the spec:

   ```bash
   jq -r '
     .proposal.specs[] as $s
     | $s.api_collections[] as $c
     | $c.apis[]?
     | "\($s.index)\t\($c.collection_data.api_interface)\t\(.name)\t\(.compute_units // "null")\t\(.block_parsing.parser_func // "null")\t\(.block_parsing.parser_arg // [] | tojson)\t\(.category.subscription // false)\t\(.category.stateful // 0)"
   ' <spec_path>
   ```

2. For each tuple, classify:
   - If method name matches an unsubscribe variant (contains `nsubscribe`) AND `category.subscription == true` → expected = 10.
   - Else if method name matches a subscribe variant (contains `ubscribe` and NOT `nsubscribe`) AND `category.subscription == true` → expected = 1000.
   - Else if `category.stateful == 1` → expected = 10.
   - Else if `parser_func == "EMPTY"` → expected = 10.
   - Else if `parser_func == "DEFAULT"` AND `parser_arg == ["latest"]` → expected = 10.
   - Else if `parser_func` is `PARSE_BY_ARG` / `PARSE_CANONICAL` / `PARSE_DICTIONARY_OR_ORDERED` → expected = 20.
   - Else: HEURISTIC (use band judgement).

3. For mechanical rows: FAIL if declared ≠ expected (exact equality).
4. For heuristic rows: FAIL only if declared is clearly outside the heuristic band.

## Return to orchestrator

```
=== GATE: cu-consistency ===
<status>  # OK | FAIL
<one FAIL row per mismatched method: method_name | declared | expected | reason>

=== SUMMARY ===
RESULT: PASS | FAIL
```

Do NOT modify the candidate spec.

END-OF-CU-CONSISTENCY-VALIDATOR-SENTINEL
