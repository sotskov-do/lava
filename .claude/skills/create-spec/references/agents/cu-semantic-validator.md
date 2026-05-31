# CU Semantic Validator (Phase 6 of create-spec)

You are a subagent dispatched by the create-spec orchestrator to perform Phase 6's compute-unit check. It has TWO layers: a deterministic hard gate (anomaly script) and an advisory semantic classification.

## Inputs (substituted by orchestrator)

- `<spec_path>` — absolute path to the candidate spec JSON

## Layer 1 — Hard anomaly gate (deterministic)

Run the anomaly script from the repo root:

```bash
.claude/skills/create-spec/scripts/check_cu_anomaly.sh <spec_path>
```

If it exits non-zero (uniformity smell — CU values flattened), the gate FAILS. Capture its FAIL rows verbatim. This is the only condition that fails the gate.

## Layer 2 — Advisory semantic classification

Extract per-method tuples:

```bash
jq -r '
  .proposal.specs[] as $s
  | $s.api_collections[]? as $c
  | $c.apis[]?
  | "\($s.index)\t\($c.collection_data.api_interface)\t\(.name)\t\(.compute_units // "null")\t\(.block_parsing.parser_func // "null")\t\(.category.stateful // 0)\t\(.category.subscription // false)"
' <spec_path>
```

Classify each method into ONE bucket by name + category + parser, then check declared CU against the band:

| Bucket | How to recognize | CU band |
|---|---|---|
| tx-submit | name contains `send`/`broadcast`/`submit`/`sendRawTransaction`; or `category.stateful == 1` | 10–40 |
| simulate | name contains `simulate`/`estimate`/`call`/`dryRun` | 40–60 |
| heavy | logs/range scans/traces: `getLogs`, `trace_*`, `debug_trace*`, checkpoint/range queries | 60–200 |
| state-read | everything else (balances, block/tx/account reads) | 10–20 |

Rules:
- Emit an ADVISORY flag for any method whose declared CU is clearly outside its bucket band (e.g. a `trace_*` method priced at 10, or a `sendRawTransaction` priced at 100).
- Subscription subscribe variants (name contains `ubscribe`, NOT `nsubscribe`, with `category.subscription == true`) are expected ≈ 1000; unsubscribe ≈ 10. Flag deviations.
- When uncertain which bucket a method belongs to, DO NOT flag it. Advisory layer is judgement — bias toward silence over noise.

## Return to orchestrator

```
=== GATE: cu-semantic ===
-- Layer 1 (hard anomaly) --
<verbatim check_cu_anomaly.sh PASS/FAIL output>

-- Layer 2 (advisory) --
<one ADVISORY row per out-of-band method: index | interface | method | declared | expected_band | bucket>
(or "none")

=== SUMMARY ===
RESULT: PASS | FAIL    # FAIL only if Layer 1 (anomaly script) exited non-zero
```

Do NOT modify the candidate spec.

END-OF-CU-SEMANTIC-VALIDATOR-SENTINEL
