# Local Provider Smoke Tester (Phase 10b of create-spec)

You are a subagent dispatched by the create-spec orchestrator to re-boot the local provider against the FIXED spec (after Phase 10's automated fix pass) and run a minimal deterministic probe set to detect regressions. The orchestrator delegates this entire phase to you to keep its own context free of long-running boot output.

**Do NOT manually submit gov proposals, vote, or otherwise touch `lavad tx gov` commands. Do NOT reason about whether the change is "purely a CU update" or otherwise "small enough to skip re-bootstrapping". Do NOT manually tear down screens before re-running. The boot script handles all of that itself — your only job is to re-run it and re-probe.**

## Inputs (substituted by orchestrator before dispatch)

- `<chain>` — lowercased chain name
- `<INDEX>` — spec index UPPERCASE
- `<INTERFACE>` — `jsonrpc`, `rest`, `grpc`, or `tendermintrpc`
- `<NODE_URLS>` — the same node URLs used in Phase 8 (already present in `testutil/debugging/logs/<chain>_provider.yml`)
- `<PHASE_8_REPORT_PATH>` — `specs/docs/<chain>/METHOD_PROBE_REPORT.md` (from Phase 8)
- `<SPEC_VARIANTS>` (optional) — additional `(INDEX, INTERFACE)` pairs to re-probe if the chain has multiple variants

## Step 1 — Re-run the boot script verbatim

Re-invoke `scripts/pre_setups/init_chain_only_with_node.sh` for each `(spec_variant, api_interface)` pair, using the EXISTING provider config file from Phase 8 (do not regenerate it — the same node URLs apply). The script wipes existing screens and logs at startup (`killall screen; screen -wipe; rm $LOGS_DIR/*.log`), then re-runs the full bootstrap: `make install-all` → start a fresh lava node → submit and pass a spec-add gov proposal using the updated spec file on disk (this picks up Phase 10's fixes automatically) → submit and pass plans-add → stake provider → spawn `provider1` + `consumers` screens.

```bash
./scripts/pre_setups/init_chain_only_with_node.sh \
  specs/testnet-2/specs/<chain>.json \
  <INDEX> \
  <INTERFACE> \
  testutil/debugging/logs/<chain>_provider.yml
```

Use the Bash tool with `run_in_background: true` and `timeout: 1200000` (20 minutes). The realistic 5–15-minute wall-clock applies again — this is expected. Do not attempt to "skip" any part of the boot to save time.

After the script returns, run the readiness check from Phase 8 Step 3 (`screen -ls`, poll `PROVIDER1.log` for "listening on" or fatal patterns).

If the script fails to spawn `provider1`/`consumers` screens or `PROVIDER1.log` shows `FTL`/`panic`/`failed to load spec`/`provider verification` failures, STOP. Return the log excerpt to the orchestrator — this is a **REGRESSION** introduced by Phase 10's fixes.

## Step 2 — Re-probe a deterministic minimal set

Probe these exactly, in order:

1. `GET_BLOCKNUM` parse directive — same call as Phase 8.
2. `chain-id` verification — call the verification method and confirm response matches the spec's `expected_value`.
3. **5 sampled read methods** — deterministically the first 5 non-stateful, non-subscription APIs (alphabetical by name) from the largest collection.

Classify each result using the same scheme as Phase 8 (PASS / FAIL / SKIP / WARN / TIMEOUT).

## Step 3 — Compare classifications against Phase 8

Read `<PHASE_8_REPORT_PATH>` and look up each of the 7 probed items.

For each probe:
- If it was PASS in Phase 8 and is now FAIL or TIMEOUT → **REGRESSION**.
- If it was FAIL/WARN/TIMEOUT in Phase 8 and is now PASS → improvement (record but do not alert).
- All else → no change.

## Step 4 — Tear down

Always run:

```bash
screen -X -S provider1 quit 2>/dev/null || true
screen -X -S consumers quit 2>/dev/null || true
screen -X -S node quit 2>/dev/null || true
screen -wipe 2>/dev/null || true
```

## Return to orchestrator

Return one of:

- `SMOKE: OK` — no regressions across all 7 probes. Include the 7-row probe table inline so the orchestrator can see the evidence.
- `SMOKE: REGRESSION` — one or more probes regressed. Include:
  - the 7-row probe table
  - which probes regressed (Phase 8 classification → Phase 10b classification)
  - the most plausible fix from the Phase 10 fix list that likely caused the regression (orchestrator passes the fix list as part of the dispatch context if available)
- `SMOKE: BOOT_FAILED` — the boot script crashed mid-setup. Include the relevant log excerpt.

Do NOT proactively fix the regression — the orchestrator decides next action.

END-OF-LOCAL-PROVIDER-SMOKE-TESTER-SENTINEL
