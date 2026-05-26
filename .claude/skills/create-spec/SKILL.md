---
name: create-spec
description: "Generate a single Lava chain spec JSON file at specs/testnet-2/specs/<chain>.json containing both mainnet and testnet entries under one proposal.specs[] array. Use when the user asks to add support for a new blockchain, create or build a chain spec, or onboard a chain to Lava. Runs a 12-phase pipeline with parallel research agents, formula-gated synthesis, autonomous jq validation, local provider boot + multi-node method probing, and worktree-isolated parallel /review-spec reviewers."
---

# Create Spec — Lava Chain Specification

This skill produces a single JSON file at `specs/testnet-2/specs/<chain>.json` that contains both mainnet and testnet spec entries under one `proposal.specs[]` array (matching the format of `specs/testnet-2/specs/iota.json`). The testnet entry imports the mainnet entry and overrides only the `chain-id` verification value.

The skill orchestrates a 12-phase pipeline. It does NOT generate documentation, governance proposals, or execute git operations. If the user asks for any of those, stop and confirm scope before continuing.

`build-spec` and `create-lava-spec` are NOT replaced by this skill — they remain on disk untouched.

## Output target

- **Path:** `specs/testnet-2/specs/<chain>.json` (lowercase filename matching the mainnet `index` lowercased — e.g. `iota.json`, `polygon.json`)
- **Structure:** single file, `proposal.title` + `proposal.description` + `proposal.specs[]` (2 entries: mainnet + testnet) + `deposit: "10001000ulava"`
- **Reference:** `specs/testnet-2/specs/iota.json` is the canonical example

## Full-read enforcement (mandatory)

Each reference file under `references/` ends with a sentinel line of the form `END-OF-<NAME>-SENTINEL`. Before each phase transition you must have observed the sentinel of the file required by that phase.

To read a reference file fully:

1. Run `wc -l <path>` to get the total line count `N`.
2. Read the file in 500-line chunks using the Read tool's `offset` parameter (1, 501, 1001, ...) until you have covered all `N` lines.
3. The final chunk MUST contain the sentinel. If you have not seen it, you have not finished — continue reading from a higher offset.
4. Do NOT begin the next phase until you have observed the sentinel.

## Phase 1 — Pre-flight

Check whether `specs/testnet-2/specs/<chain>.json` already exists, where `<chain>` is the lowercased mainnet index the user wants to add.

Run:

```bash
ls specs/testnet-2/specs/<chain>.json 2>/dev/null
```

- If the file exists, ask the user: "Use as base / adapt / scratch?" Do not overwrite without explicit confirmation.
- If it does not exist, proceed to Phase 2.

## Phase 2 — Gather inputs

Ask the user only for what they alone can decide. Do not guess defaults:

- **Chain name** (e.g., "Iota")
- **Mainnet spec index** (uppercase, 3–10 chars, e.g., `IOTA`)
- **Testnet spec index** (uppercase, e.g., `IOTAT` or `IOTAS`)
- **Docs URL** (optional — Phase 3 will pick one and report if missing)
- **Public RPC URLs** (optional — Phase 3 will pick 2-3 each for mainnet and testnet if missing)
- **Inheritance hint** (optional — e.g., "EVM-compatible, imports ETH1")

If the user is vague ("add Polygon"), ask. Don't proceed until you have at minimum the chain name, mainnet index, and testnet index.

## Phase 3 — Parallel research (4 background agents)

Before dispatching, read `references/phase1-research.md` end-to-end (full-read, observe `END-OF-PHASE1-SENTINEL`). It contains the blockchain-analysis framework, third-party-API decision tree, index-naming conventions, and API-discovery patterns that inform how to brief the research agents. Subagents will not read this file themselves — you (the orchestrator) extract the relevant context from it and weave it into each agent prompt's `{chain_name}`, `{docs_url}`, etc. substitutions.

Dispatch four research agents in parallel via a SINGLE message with four Agent tool calls. Each agent runs in the background (`run_in_background: true`) and uses `subagent_type: general-purpose`.

Read the four agent prompt files first (full-read with sentinel verification, where applicable):

- `.claude/skills/create-spec/references/agents/api-docs-researcher.md`
- `.claude/skills/create-spec/references/agents/chain-metadata-researcher.md` (observe `END-OF-AGENT-CHAIN-METADATA-SENTINEL`)
- `.claude/skills/create-spec/references/agents/upstream-spec-scout.md`
- `.claude/skills/create-spec/references/agents/plugin-researcher.md`

Substitute placeholders (`{chain_name}`, `{docs_url}`, `{mainnet_indices_or_known_parents}`, `{public_repo_path}`) with the values gathered in Phase 2 plus any heuristics (e.g., `{public_repo_path}` is empty unless the user has resolved a lava-specs clone).

Dispatch all four in a single message:

```
Agent(description: "Research api-docs for {chain}", subagent_type: "general-purpose", run_in_background: true, prompt: <api-docs-researcher.md with placeholders substituted>)
Agent(description: "Research chain metadata for {chain}", subagent_type: "general-purpose", run_in_background: true, prompt: <chain-metadata-researcher.md with placeholders substituted>)
Agent(description: "Find upstream parent spec for {chain}", subagent_type: "general-purpose", run_in_background: true, prompt: <upstream-spec-scout.md with placeholders substituted>)
Agent(description: "Detect plugins/extensions for {chain}", subagent_type: "general-purpose", run_in_background: true, prompt: <plugin-researcher.md with placeholders substituted>)
```

When all four agents complete (you will receive notifications), collect their reports and proceed to Phase 4.

If the upstream-spec-scout agent reports that no lava-specs clone was resolved, treat its output as empty (no parent-spec hints) and continue.

## Phase 4 — Synthesis (gated by phase-file reads)

Before constructing any spec JSON, you must observe sentinels for these reference files in this order:

1. `references/phase2-network-params.md` → observe `END-OF-PHASE2-SENTINEL`
2. `references/phase3.1-inheritance.md` → observe `END-OF-PHASE3.1-SENTINEL`
3. `references/phase3.2-api-methods-configuration.md` → observe `END-OF-PHASE3.2-SENTINEL`
4. `references/phase3.3-api-collections.md` → observe `END-OF-PHASE3.3-SENTINEL`
5. `references/phase3.4-parse-directives-and-extensions.md` → observe `END-OF-PHASE3.4-SENTINEL`
6. `references/appendix-reference-tables.md` → observe `END-OF-APPENDIX-SENTINEL`
7. `references/common-pitfalls.md` → observe `END-OF-PITFALLS-SENTINEL`

Then before writing any JSON, emit a **calculations table** to the user showing every derived network parameter and the math behind it:

| Parameter | Source | Formula | Computed value |
|---|---|---|---|
| `average_block_time` | (docs / empirical measurement — cite which) | — | (ms) |
| `block_distance_for_finalized_data` | (consensus type — PoW=6–12, BFT=1–3, instant=1) | — | (int) |
| `blocks_in_finalization_proof` | derived | `max(ceil(1000 / average_block_time), 3)` | (int) |
| `allowed_block_lag_for_qos_sync` | derived | `max(ceil(10000 / average_block_time), 1)` | (int) |
| `reliability_threshold` | standard | `268435455` | `268435455` |
| `data_reliability_enabled` | standard | `true` | `true` |

**Block-time tie-breaker rule (C):** Determine `average_block_time` by this exact priority:

1. **If docs publish a single canonical value** (one number, not a range): **USE THE DOCS VALUE** unless empirical measurement disagrees by MORE than 20%. Drift up to 20% is normal RPC jitter and is NOT a reason to deviate. (For example, if empirical = 3800ms and docs = 4000ms, lock **4000** — the 5% drift is jitter.)
2. **If docs publish a range** (e.g., "X–Y ms"): use the **lower bound of the range** OR the empirical value, **whichever is lower**. Never round UP "for safety" or "conservatively" — `average_block_time` directly multiplies into `blocks_in_finalization_proof` and `allowed_block_lag_for_qos_sync` via the formulas above, and rounding up cascades into wrong downstream values. (For example, if empirical = 219ms and docs say 200–250ms, lock **200**, not 250.)
3. **If docs are silent**: use the empirical median directly.
4. **If empirical and a single docs value disagree by >20%**: ask the user which to trust — do not silently pick one.

After the user has had a chance to challenge the table, construct draft JSON applying these strict synthesis rules:

- **NEVER extract spec content from git history.** You (the orchestrator) MUST NOT run `git show <commit>:specs/...`, `git log -p -- specs/...`, `git restore --source=<commit> specs/...`, or any similar command to retrieve the contents of a spec that previously existed in this repo but is no longer in the working tree. The `upstream-spec-scout` agent is bound by the same rule (see `references/agents/upstream-spec-scout.md`). Two reasons: (1) **Evaluation bias** — when this skill is being evaluated, the "gold" spec being scored against is frequently the recently-deleted file one or two commits back; recovering it via git makes the candidate-vs-gold comparison circular and invalidates the score. (2) **Staleness** — a deleted spec was deleted for a reason, usually because it was wrong; recovering it bakes the defects back in. If the scout's report mentions "X previously existed in git history", treat that as a name-level note only — do NOT go retrieve the file yourself. Build from the chain's current docs + sibling-spec templates in the working tree (e.g., `sui.json` for IOTA).
- **REJECT all agent "trim", "scope", "exclude", or "narrow" recommendations.** Research agents (api-docs-researcher in particular) may suggest reducing the method set with framing like "scoping suggestions trim to ~50" or "consider excluding the foo_* family". You MUST ignore these suggestions. The full discovered method list is the input to synthesis. Apply only the explicit-omission rule below — never the agent's opinion.
- **Method-set input = UNION of api-docs-researcher AND upstream-spec-scout (A).** The synthesis input is the union of: (1) every method `api-docs-researcher` discovered from chain docs/WebSearch, AND (2) every method `upstream-spec-scout` found in any existing spec (deleted-from-branch, sister-ecosystem template like `sui.json` for IOTA, prior version in `specs/mainnet-1/`, or any matching upstream). If the scout found a method that the researcher didn't, **INCLUDE IT** — existing-spec evidence is higher quality than fresh web search. The only valid reason to omit a scout-found method is an empirical curl proving the method no longer exists on the chain (`-32601 method not found` against the public RPC). "Researcher didn't find it" is NOT a valid omission reason.
- **All methods from chain docs MUST appear in the spec.** Take the COMPLETE method list (union of researcher + scout) and include every method. The only acceptable reasons to omit are documented in the chain's API reference itself: explicitly marked deprecated, explicitly internal/admin-only, or explicitly platform-specific (e.g., GraphQL-only on a JSON-RPC spec). "The agent suggested trimming" is NOT a valid reason.
- **Subscription methods belong in MAIN, not in an add-on (B).** Methods with `category.subscription: true` (subscribe/unsubscribe pairs) live in the **same collection as the chain's core read API**, NOT in a separate `add_on: "indexer"` collection. The `indexer` add-on is for methods that require an external indexer service running (e.g., metrics aggregations like `iotax_getNetworkMetrics`, `iotax_getMoveCallMetrics`, address rollups, epoch rollups). Methods served by every regular full node — including dynamic-fields queries, owned-objects, query-events, query-transactions, and ALL subscriptions — belong in MAIN. Cross-check the scout's findings: if the scout's template spec has a method in MAIN, KEEP IT IN MAIN.
- **Parse-directive completeness for subscriptions (D).** For every API with `category.subscription: true`:
  - Subscribe variants (method name contains `ubscribe` but NOT `nsubscribe`) → MUST have a matching `parse_directive` entry with `function_tag: "SUBSCRIBE"` and `api_name: "<that method's name>"` in the same collection.
  - Unsubscribe variants (method name contains `nsubscribe`) → MUST have a matching `parse_directive` entry with `function_tag: "UNSUBSCRIBE"`, an explicit `function_template` (e.g., `"{\"jsonrpc\":\"2.0\",\"method\":\"<name>\",\"params\":[\"%s\"],\"id\":1}"`), and `api_name: "<that method's name>"`.
  - Without these parse_directives, the methods are listed but the relay layer cannot route them — they are effectively broken.
- **Parse-directives, extensions, and verifications follow the references — not the template (F).** The canonical structures (function tags, function_template shapes, result_parsing patterns, archive/pruning encoding) live in `references/phase3.4-parse-directives-and-extensions.md` and `references/appendix-reference-tables.md`. You already observed their sentinels in Phase 4 — use that content as the source of truth. A template spec (when `upstream-spec-scout` found one) is a **concrete syntax example** for chains in the same ecosystem — useful for copying the exact `function_template` arg shape for non-obvious cases (e.g., `params: [null, 1, false]` for `GET_EARLIEST_BLOCK` in Sui/IOTA). The reference dictates WHICH elements must exist; the template (if any) shows what they look like. Completeness is enforced by the Phase 6 gates, not by template diff.
- **Multi-collection splits.** Many chains DO have a legitimate add-on collection (e.g., IOTA's `iotax_*` address-metrics methods, EVM's `debug_*`/`trace_*` add-ons). Add an `add_on` collection ONLY when the methods require external infrastructure (indexer service, archive node, trace database). Default everything else to MAIN.
- **Every addon and extension has a matching `verifications` block.** An archive extension requires a `pruning` verification with `GET_EARLIEST_BLOCK`. An `add_on` collection requires its own verification (e.g., the indexer collection verifies via `iotax_getTotalTransactions` returning `*`).
- **SUBSCRIBE and UNSUBSCRIBE methods share the same `local` and `stateful` flags as each other** (typically both `local: false, stateful: 0, subscription: true`).
- If `imports` is set AND the child's `average_block_time` is materially faster than the parent's: **explicitly override** the inherited `archive.rule.block` and `pruning.latest_distance` in the child's main collection. The parent's values are calibrated for the parent's block rate — silently inheriting them produces a wrong pruning window for the child. (For example, a chain with 1s blocks importing `ETH1` would silently inherit ETH1's archive/pruning sizing that was calibrated for 12s blocks — the resulting archive window is ~12× too short.)
- `stateful: 1` ONLY for state-modifying broadcasts. Read-only helpers like `eth_call`, `eth_estimateGas`, `eth_fillTransaction`, `debug_traceCall` are `stateful: 0` even when they take tx-shaped args.
- Every API with `category.hanging_api: true` has an explicit `timeout_ms` set. The `hanging_api` flag alone only adds `2 * average_block_time` to the relay timeout — insufficient on fast chains.

### Per-method `block_parsing` inference (H)

For every method, infer `block_parsing` from the method's argument signature — do NOT default every method to `DEFAULT` / `["latest"]`. Wrong `block_parsing` produces wrong CU values via rule E below, AND breaks block-aware routing at relay time. Map each method through this table:

| Argument shape | `parser_func` | `parser_arg` |
|---|---|---|
| Method takes a positional state-selecting identifier (block number, ledger index, checkpoint sequence, object ID, tx hash) — i.e., the arg's value selects WHICH historical state is queried | `PARSE_BY_ARG` | `["<position-index>"]` (e.g., `["0"]` for the first param) |
| Method takes a request object with a nested state-selecting field | `PARSE_CANONICAL` | dotted path to that field (e.g., `["params", "ledger_index"]`) |
| Method takes NO state-selecting arg AND returns current-state data (latest balance, current fee, latest block) | `DEFAULT` | `["latest"]` |
| Method takes a tag like `"latest" \| "earliest" \| "pending"` or a block-or-hash union | `PARSE_DICTIONARY_OR_ORDERED` | the position index or path |
| Method takes NO args and returns static / computational data (genesis info, network constants, node identity, chain ID) | `EMPTY` | `[""]` |

Cross-check against `references/phase3.2-api-methods-configuration.md` (you already observed `END-OF-PHASE3.2-SENTINEL` in this phase) for the canonical mapping per parser_func.

### CU value rules (E) — mechanical from block_parsing + category

Use this table to assign `compute_units`. The table is exhaustive — do NOT apply "generic CU bands" from memory; map every method through these rules:

| Method shape | CU |
|---|---|
| `block_parsing.parser_func == "EMPTY"` (static, no chain state) | 10 |
| `block_parsing.parser_func == "DEFAULT"` with `parser_arg == ["latest"]` (simple read of current state) | 10 |
| `block_parsing.parser_func == "PARSE_BY_ARG"` OR `"PARSE_CANONICAL"` (state-by-id query — fetch object/tx/checkpoint by hash or sequence number) | 20 |
| `block_parsing.parser_func == "PARSE_DICTIONARY_OR_ORDERED"` (block-or-tag query, e.g., EVM `eth_getBlockByNumber`) | 20 |
| `category.subscription: true` AND method name is a **subscribe** variant (name contains `ubscribe` but NOT `nsubscribe`) | 1000 |
| `category.subscription: true` AND method name is an **unsubscribe** variant (name contains `nsubscribe`) | 10 |
| `category.stateful: 1` (broadcast/state-modifying) | 10 |
| Heavy compute (full-scan, `getLogs`-style) when explicitly classified as such by api-docs-researcher | 60–100 |
| Traces / debug_* | 100–200 |

If a method falls outside this table, default to 10 and flag to the user. **`unsubscribe` is not a subscribe — never give it CU=1000.** **State-by-id reads are not simple reads — never give them CU=10.**

### Before writing JSON (G): mandatory pre-write summary, refuse-to-write gate

Print to the user this exact summary BEFORE calling Write on the spec file:

```
Methods discovered by api-docs-researcher: <N_researcher>
Methods found by upstream-spec-scout:      <N_scout>
Union (deduplicated):                       <N_union>
Methods included in spec:                   <M> (split: main=<X>, <addon1>=<Y>, ...)
```

**Refuse-to-write gate.** If `M < N_union`, you MUST list every omitted method with an explicit reason from the allowed set: `deprecated` / `admin-only` / `platform-specific (e.g., GraphQL-only)` / `empirically absent (curl returned -32601 against <node_url>)`. If any omission lacks a documented reason from this set, ADD THE METHOD BACK and re-print the summary before proceeding. Do NOT call the Write tool until either `M == N_union` OR every gap is justified with one of the four allowed reasons.

After the summary prints and any gaps are reconciled, validate one more shape detail before Write:

- For every method with `category.subscription: true`, confirm a matching `parse_directive` exists in the same collection (rule D). If any subscription method lacks its parse_directive, ADD IT before writing.

Then call Write.

## Phase 5 — Inheritance audit (CONDITIONAL)

Skip this phase entirely if the mainnet draft's `imports` array is empty.

If `imports != []`, perform the two-step audit from `references/phase3.1-inheritance.md` (you should have already observed its sentinel in Phase 4):

**Step 1 — Parent's APIs vs chain's documented APIs.** Use `jq` to extract parent method names and `comm -23` to diff:

```bash
PARENT="ETH1"  # or whatever the import is
PARENT_FILE="specs/mainnet-1/specs/${PARENT,,}.json"
[ -f "$PARENT_FILE" ] || PARENT_FILE="specs/testnet-2/specs/${PARENT,,}.json"
jq -r '.proposal.specs[] | select(.index == "'$PARENT'") | .api_collections[].apis[].name' "$PARENT_FILE" | sort -u > /tmp/parent_methods.txt
# Compare against chain-docs methods (from api-docs-researcher report); write its method list to /tmp/chain_methods.txt
comm -23 /tmp/parent_methods.txt /tmp/chain_methods.txt > /tmp/ghosts.txt
```

For each "ghost" method (in parent but not chain docs), run an empirical curl probe against the chain's public RPC:

```bash
curl -s -X POST -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"<ghost>","params":[],"id":1}' \
  <chain_rpc_url>
```

If the response is `-32601 method not found` → the method is a ghost, disable or remove it in the child spec. If the response is anything else → method exists; retain inheritance.

**Step 2 — Chain-specific additions.** Diff chain docs against parent:

```bash
comm -13 /tmp/parent_methods.txt /tmp/chain_methods.txt > /tmp/additions.txt
```

Every method in `/tmp/additions.txt` MUST appear in the child spec. Commonly missed categories: `admin_*`, `txpool_*`, `*_Sync` variants.

Output the diff results and probe results verbatim to the user before proceeding.

## Phase 6 — Completeness checklist

Walk the extended completeness checklist below. For each item, either confirm or fix inline before proceeding to Phase 7. Items marked **GATE** are hard refuse-to-write gates: if not satisfied, you MUST NOT call Write until they are fixed.

- `index` is uppercase, unique, matches the chain
- `name`, `enabled`, `min_stake_provider`, `shares` present at top level of each spec entry
- `average_block_time` sourced from docs OR empirically measured (cite which)
- `block_distance_for_finalized_data` sourced from official finality docs
- `blocks_in_finalization_proof` = `max(ceil(1000 / average_block_time), 3)` — verify computed value matches Phase 4 table
- `allowed_block_lag_for_qos_sync` = `max(ceil(10000 / average_block_time), 1)` — verify computed value matches Phase 4 table
- `reliability_threshold: 268435455`, `data_reliability_enabled: true`
- If `imports` is set, Phase 5's audit was performed and its outputs were shown to the user
- All methods from api-docs-researcher's report appear in the spec
- Every addon has a matching `verifications` block
- SUBSCRIBE / UNSUBSCRIBE share `local` and `stateful` flags
- Stateful APIs only mark broadcast/state-modifying methods
- Every `hanging_api: true` API has an explicit `timeout_ms`
- Every API has `name`, `enabled`, `compute_units`, `block_parsing`, `category`
- `chain-id` `expected_value` obtained from a **live curl** against the mainnet RPC (not converted from a docs decimal)
- Testnet entry's `chain-id` `expected_value` obtained from a live curl against the testnet RPC
- Every `parser_arg` is an array of strings
- No duplicate API `name` within a single collection

### GATE — Archive ↔ pruning ↔ GET_EARLIEST_BLOCK consistency

For each spec entry: `archive` extension, `pruning` verification, and `GET_EARLIEST_BLOCK` parse_directive are an indivisible triplet. They are all present or all absent. The canonical structure of each lives in `references/phase3.4-parse-directives-and-extensions.md`.

```bash
CAND=specs/testnet-2/specs/<chain>.json
jq '.proposal.specs[] | {
  index,
  archive_ext:        ([.api_collections[].extensions[]?.name] | contains(["archive"])),
  pruning_ver:        ([.api_collections[].verifications[]?.name] | contains(["pruning"])),
  earliest_directive: ([.api_collections[].parse_directives[]?.function_tag] | contains(["GET_EARLIEST_BLOCK"]))
}' "$CAND"
```

For each entry, the three booleans must be all `true` or all `false`. Any mixed row → STOP, fix by reading `references/phase3.4-...md` and adding the missing element(s). Do NOT call Write while mixed.

### GATE — Template-shape diff (if scout returned a template)

If `upstream-spec-scout` recommended a template spec from the working tree, run a structural diff to catch silent drops. Skip this gate when there is no template.

```bash
TPL=<template-path>   # e.g., specs/testnet-2/specs/sui.json
for axis in 'parse_directives[]?.function_tag' 'extensions[]?.name' 'verifications[]?.name'; do
  echo "--- $axis ---"
  diff <(jq -r "[.proposal.specs[0].api_collections[].${axis}] | unique[]" "$TPL") \
       <(jq -r "[.proposal.specs[0].api_collections[].${axis}] | unique[]" "$CAND")
done
```

Any `<` line is a template element missing from the candidate — restore per references. Any `>` line is candidate-only and must be defensible (chain-specific extension, justified by the chain's docs).

For the chain-id curl step, run this for both mainnet and testnet:

```bash
curl -s -X POST -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
  <mainnet_rpc_url>
# For non-EVM chains, use the chain's equivalent (e.g., iota_getChainIdentifier).
```

Capture the returned hex value VERBATIM and put it in the spec's `verifications[].values[0].expected_value`. Show the response to the user. Do not convert from a decimal in the docs — hex/decimal typos are a common spec-failure cause.

### GATE — Live parse-directive validation

A `parse_directive` is more than a structural element: its `function_template` and `result_parsing` together specify a CALL + EXTRACTION pipeline that the relay layer executes at runtime. Wrong template or wrong result_parsing path looks fine in jq but fails at relay time. For each `parse_directive` in the candidate, issue the call against the chain's mainnet public RPC and verify the extraction:

For each directive (skip if no mainnet RPC URL is available — Phase 8 will catch the rest):

1. Issue the `function_template` exactly as written (substitute `%d` / `%s` placeholders with a reasonable value — the latest block number for `GET_BLOCK_BY_NUM`, a dummy subscription ID for `UNSUBSCRIBE`).
2. Capture the response.
3. Walk through `result_parsing.parser_arg` using `result_parsing.parser_func` semantics (consult `references/appendix-reference-tables.md` for the exact semantics of `PARSE_BY_ARG`, `PARSE_CANONICAL`, `PARSE_DICTIONARY_OR_ORDERED`, `EMPTY`, `DEFAULT`).
4. Verify the extracted value type matches the directive's `function_tag`:
   - `GET_BLOCKNUM` → must extract a positive integer or hex-int (whatever encoding the directive declares)
   - `GET_BLOCK_BY_NUM` → extraction target is typically a block hash / digest — must extract a string-typed identifier
   - `GET_EARLIEST_BLOCK` → must extract a positive integer ≤ the `GET_BLOCKNUM` result
   - `VERIFICATION` (chain-id) → extracted value MUST match the verification's `expected_value`
   - `SUBSCRIBE` / `UNSUBSCRIBE` → cannot fully validate without WebSocket (Phase 8 covers this); structural-only check here

Show the user, for each directive: (a) the request issued, (b) the raw response, (c) the extracted value, (d) PASS/FAIL.

If any directive FAILS:
- **Wrong `api_name`** (response is `-32601` method not found): swap the api_name; the orchestrator MUST cross-check against the chain's documented method that actually returns the needed data.
- **Wrong `parser_arg` path** (extraction yields `null` or wrong-typed value): re-walk the response and correct the path.
- **Wrong `function_template`** (response is parse error or empty): correct the JSON-RPC params shape.
- **Wrong directive choice entirely** (e.g., a method returns node-local data instead of chain-wide data): pick a different `api_name` that returns the right kind of data.

Do NOT proceed to Phase 7 with any FAIL outstanding.

## Phase 7 — Write & autonomous jq validation

Write the single file `specs/testnet-2/specs/<chain>.json` using the Write tool. The file structure must match `specs/testnet-2/specs/iota.json`:

```json
{
  "proposal": {
    "title": "Add Specs: <CHAIN>",
    "description": "<one-sentence description>",
    "specs": [
      { "index": "<CHAIN>", "name": "<chain> mainnet", "imports": [], ... },
      { "index": "<CHAIN_T>", "name": "<chain> testnet",
        "imports": ["<CHAIN>"],
        "api_collections": [{ ..., "apis": [], "verifications": [{ "name": "chain-id", "values": [{ "expected_value": "<testnet_hex>" }] }] }] }
    ]
  },
  "deposit": "10001000ulava"
}
```

Then run `jq` autonomously and report the result:

```bash
jq . specs/testnet-2/specs/<chain>.json > /dev/null
echo "jq exit: $?"
```

If exit is non-zero, capture the error excerpt:

```bash
jq . specs/testnet-2/specs/<chain>.json 2>&1 | head -n 20
```

Fix the JSON and re-run until exit 0. Do not proceed to Phase 8 until `jq` exits 0.

## Phase 8 — Local provider boot + multi-node method probe

**Execute the steps below verbatim. Do NOT inspect `scripts/pre_setups/init_chain_only_with_node.sh` to reason about its internals, do NOT question the timeouts, and do NOT improvise the config format. The procedure below is authoritative.**

Optional informational read for additional context only (the steps below take precedence over anything you find here): `references/phase4-testing-and-validation.md` (observe `END-OF-PHASE4-SENTINEL`).

The boot script does a full local-chain bootstrap (compile binaries → start a fresh lava node → submit and pass a spec-add gov proposal → submit and pass a plans-add proposal → stake provider → advance an epoch → spawn `provider1` and `consumers` `screen` sessions). Realistic wall-clock: **5–15 minutes per invocation**. This is expected. Do not abort early.

### Step 8a — Write the provider config

Write `testutil/debugging/logs/<chain>_provider.yml` with EXACTLY this structure (no other fields, no other sections):

```yaml
# ./scripts/pre_setups/init_chain_only_with_node.sh specs/testnet-2/specs/<chain>.json <INDEX> <interface> testutil/debugging/logs/<chain>_provider.yml
endpoints:
  - api-interface: <INTERFACE>
    chain-id: <INDEX>
    network-address:
      address: 127.0.0.1:2220
    node-urls:
      - url: <NODE_URL_1>
      - url: <NODE_URL_2>
      - url: <NODE_URL_3>
```

Rules — apply exactly:
- `network-address` is a nested object with a single `address:` field. Value is **always** `127.0.0.1:2220` (the init script hardcodes this listener; do not change it).
- `<INTERFACE>` is one of: `jsonrpc`, `rest`, `grpc`, `tendermintrpc`. Use the spec's `api_collections[].collection_data.api_interface` value.
- `<INDEX>` is the spec index — uppercase (e.g., `IOTA`, `IOTAT`).
- `node-urls` is a flat list of `- url: <URL>` items. **Add 2–3 entries**, one per node URL the user or chain-metadata-researcher selected.
- For chains with WebSocket subscriptions, add the `wss://` URL as ANOTHER entry in the same `node-urls` list (do NOT create a separate section). Example: `- url: wss://eth-rpc.example.com`.
- For multi-interface chains (Cosmos has jsonrpc + rest + tendermintrpc + grpc), repeat the `endpoints[]` block once per interface, all using the same `network-address.address: 127.0.0.1:2220`. See `testutil/debugging/logs/cosmoshub_provider.yml` for the multi-interface shape.
- `add_on` / addons are defined in the SPEC file (`collection_data.add_on`), NOT in this provider config. Do not invent addon fields here.

After writing, dump the file and confirm it parses as YAML:

```bash
cat testutil/debugging/logs/<chain>_provider.yml
python3 -c 'import yaml,sys; yaml.safe_load(open("testutil/debugging/logs/<chain>_provider.yml"))'
```

### Step 8b — Invoke the boot script

Run the script in the FOREGROUND (the script itself daemonizes the provider via `screen`; you must wait for the script to finish its setup work before probing).

```bash
./scripts/pre_setups/init_chain_only_with_node.sh \
  specs/testnet-2/specs/<chain>.json \
  <INDEX> \
  <INTERFACE> \
  testutil/debugging/logs/<chain>_provider.yml
```

Use the Bash tool's `run_in_background: true` option, set `timeout: 1200000` (20 minutes), and watch for completion. Do NOT use a 60s timeout. Do NOT redirect to `/tmp/provider_*.log` — the script writes its real output to `testutil/debugging/logs/PROVIDER1.log` and `testutil/debugging/logs/CONSUMERS.log` (it `rm`s these at startup and re-creates them).

### Step 8c — Wait for the provider to be ready

The script returns control once it has spawned the `provider1` and `consumers` `screen` sessions. After it returns, the provider is starting up but may not yet be listening. Confirm readiness:

```bash
# 1. Both screen sessions exist
screen -ls | grep -E "(provider1|consumers)"
# Expected: two lines, one per session

# 2. Provider has bound its listener
timeout 60 bash -c 'until grep -q "listening on" testutil/debugging/logs/PROVIDER1.log 2>/dev/null \
  || grep -qE "FTL|panic|failed to load spec|provider verification" testutil/debugging/logs/PROVIDER1.log 2>/dev/null; do
  sleep 2
done'

# 3. Print last 50 lines of provider log for evidence
tail -n 50 testutil/debugging/logs/PROVIDER1.log
```

If the log shows `FTL`, `panic`, `failed to load spec`, or `provider verification` failures before any success marker, STOP. Capture the full log, present the error to the user, and go to Step 8f (teardown) before deciding next action. Do NOT skip to Step 8d.

If `screen -ls` shows no `provider1` session, the script crashed mid-setup. STOP and present the script's stdout/stderr to the user.

### Step 8d — Method-probe loop

For every API in every collection of the current spec variant:

| Category | Probe action |
|---|---|
| `category.stateful: 1` | SKIP. Record reason: "stateful — would broadcast transaction". |
| `category.subscription: true` | Open WS to `<NODE_URL>` (use the `wss://` entry from the provider config), send the subscribe call with sample params from the spec's `parse_directive` or `block_parsing` hints, **wait up to 30 seconds for at least one message**, then send unsubscribe. PASS = ≥1 message received. FAIL = timeout. |
| Anything else | Build the simplest valid call from `block_parsing` + `parse_directive` hints. Send it directly to each of the 2–3 `node-urls` URLs (NOT through the lava consumer). Classify. |

Response classification:
- Response with `result` field (any value, including empty) → **PASS** (method exists and responded).
- Response with `error.code == -32601` → **FAIL** (method does not exist on chain).
- Response with `error.code == -32602` (invalid params) → **PASS-existence** (method exists; full functional probe would need correct args).
- Response with any other `error.code` → **WARN** (record code + message).
- Timeout (no response in 10s) → **TIMEOUT**.
- Node disagreement (2-3 nodes return materially different shapes for the same method) → **WARN-DISAGREEMENT** (record which nodes disagree).

### Step 8e — Write the probe report

```bash
mkdir -p specs/docs/<chain>
```

Write `specs/docs/<chain>/METHOD_PROBE_REPORT.md`:

```markdown
# Method Probe Report — <chain>

Generated: <UTC timestamp>
Provider config: testutil/debugging/logs/<chain>_provider.yml
Spec variant: <INDEX> (<INTERFACE>)
Nodes probed: <URL_1>, <URL_2>, <URL_3>

| Method | Classification | Node 1 | Node 2 | Node 3 | Notes |
|---|---|---|---|---|---|
| <method> | <PASS/FAIL/SKIP/WARN/TIMEOUT> | <code> | <code> | <code> | <one-line note> |
| ... |
```

### Step 8f — Tear down

```bash
screen -X -S provider1 quit 2>/dev/null || true
screen -X -S consumers quit 2>/dev/null || true
screen -X -S node quit 2>/dev/null || true
screen -wipe 2>/dev/null || true
```

Repeat 8a–8f for each `(spec_variant, api_interface)` pair. Each iteration starts from a fresh lava node (the init script wipes screens and logs at startup).

## Phase 9 — Parallel reviewers (3 fresh subagents, immediate-rename for collision)

**Do NOT use `isolation: "worktree"`.** Worktrees are created via `git worktree add`, which checks out HEAD — the last committed state. Since this skill never commits, the new candidate spec written by Phase 7 is uncommitted and would NOT be visible inside a worktree. A reviewer in a worktree would review the previously-committed (stale) spec, not the candidate. This produces phantom CRITICAL findings with line references outside the real file. Anchoring isolation is achieved by fresh-subagent-context alone, not by filesystem separation.

**Before dispatching:** clear any prior parallel-review report files so the reviewers start clean:

```bash
mkdir -p specs/docs/<chain>
rm -f specs/docs/<chain>/SPEC_REVIEW_GAPS.md
rm -f specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_*.md
```

Dispatch THREE Agent subagents in parallel via a SINGLE message, each with `subagent_type: general-purpose` and NO `isolation` parameter. Each subagent receives an `N` value (1, 2, or 3) so it knows which numbered output file to write. The prompt for reviewer N:

> You are reviewing a Lava blockchain spec. Your reviewer index is **N** (used in the output filename below).
>
> Run the `/review-spec` skill on `specs/testnet-2/specs/<chain>.json`. Pass through `$ARGUMENTS[1]` (API docs path, may be empty) and `$ARGUMENTS[2]` (credentials path, may be empty).
>
> Before running `/review-spec`, read `specs/docs/<chain>/METHOD_PROBE_REPORT.md` if it exists and incorporate the probe findings into your review (especially any FAIL or WARN classifications).
>
> `/review-spec` writes its report to the hard-coded path `specs/docs/<chain>/SPEC_REVIEW_GAPS.md`. **As the LAST step of your work — immediately after `/review-spec` returns** — rename that file to a unique numbered path so the other parallel reviewers do not clobber it:
>
> ```bash
> mv -n specs/docs/<chain>/SPEC_REVIEW_GAPS.md specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_N.md
> ```
>
> Use `mv -n` (no clobber) — if the destination already exists, the move fails rather than overwriting another reviewer's work. After the `mv`, verify it succeeded:
>
> ```bash
> test -f specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_N.md && echo "RENAMED_OK" || echo "RENAMED_FAIL"
> ```
>
> If the rename failed (destination already existed OR source didn't exist because another reviewer's parallel write clobbered yours), retry your `/review-spec` invocation once. Then attempt the rename again.
>
> Return:
> 1. The FULL contents of `specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_N.md` as the body of your response.
> 2. On the LAST line of your response, print exactly: `TALLY: CRITICAL=<X> MEDIUM=<Y> MINOR=<Z>` with integer counts.
>
> Do not print anything after the TALLY line.

After all three subagents return, in the primary working tree:

1. Parse each subagent's TALLY line. If any TALLY is missing or unparseable, abort and report which reviewer.
2. Verify all three files exist:
   ```bash
   ls -la specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_{1,2,3}.md
   ```
   If any are missing, the race-condition rename failed for that reviewer. Re-dispatch JUST the missing reviewer index and wait for it to complete (sequential at this point — collision risk is gone because only one reviewer is running).
3. The reports are now on disk at their numbered paths; no further extraction needed.

**Sanity check after collection:** if any reviewer reports CRITICAL findings whose `evidence_line_number` exceeds the actual line count of `specs/testnet-2/specs/<chain>.json`, that reviewer reviewed stale state — likely because the candidate file was modified after the reviewer started. Note the discrepancy to the user and either re-dispatch that one reviewer, or treat its findings as advisory rather than authoritative. Verify with:

```bash
wc -l specs/testnet-2/specs/<chain>.json
```

## Phase 10 — Synthesize gaps + single fix pass

Read all three parallel-reviewer reports. Build a deduplicated list of CRITICAL + MEDIUM gaps, keyed by `(gap_title, evidence_line_number)`. Drop MINOR gaps (they are out of scope for the automated fix pass).

Snapshot the spec before fixing:

```bash
cp specs/testnet-2/specs/<chain>.json /tmp/spec_<chain>_pre_fix.json
```

Dispatch one `general-purpose` Agent subagent (no worktree needed — main filesystem) with this prompt:

> You are fixing a Lava blockchain spec. Read `specs/testnet-2/specs/<chain>.json` and the deduplicated gap list below. Apply EVERY listed CRITICAL and MEDIUM fix in one pass. Do not touch any field not mentioned in the gap list. Do not refactor, reformat, or improve adjacent fields.
>
> [paste deduplicated gap list with file:line citations and recommended values]
>
> Return a markdown summary of every change in the format:
> `- <file>:<line> — <one-sentence description> (gap: <severity>, "<gap title>")`

After the fixer returns, validate JSON again:

```bash
jq . specs/testnet-2/specs/<chain>.json > /dev/null
echo "jq exit: $?"
```

If exit non-zero: outcome = `BROKEN_AFTER_FIX`. Present the snapshot path (`/tmp/spec_<chain>_pre_fix.json`), the `jq` error, and the fixer's diff to the user. STOP. Do not proceed to Phase 10b.

## Phase 10b — Smoke regression test

**Do NOT manually submit gov proposals, vote, or otherwise touch `lavad tx gov` commands.** Do NOT reason about whether the change is "purely a CU update" or otherwise "small enough to skip re-bootstrapping". Do NOT manually tear down screens before re-running. The boot script handles all of that itself — your only job is to re-run it and re-probe.

### Step 10b.1 — Re-run the boot script verbatim

Re-invoke `scripts/pre_setups/init_chain_only_with_node.sh` for each `(spec_variant, api_interface)` pair, using the EXISTING provider config file from Phase 8 (do not regenerate it — the same node URLs apply). The script wipes existing screens and logs at startup (`killall screen; screen -wipe; rm $LOGS_DIR/*.log`), then re-runs the full bootstrap: `make install-all` → start a fresh lava node → submit and pass a spec-add gov proposal using the updated spec file on disk (this picks up Phase 10's fixes automatically) → submit and pass plans-add → stake provider → spawn `provider1` + `consumers` screens.

```bash
./scripts/pre_setups/init_chain_only_with_node.sh \
  specs/testnet-2/specs/<chain>.json \
  <INDEX> \
  <INTERFACE> \
  testutil/debugging/logs/<chain>_provider.yml
```

Use the Bash tool with `run_in_background: true` and `timeout: 1200000` (20 minutes). Same as Phase 8b. The realistic 5–15-minute wall-clock applies again — this is expected. Do not attempt to "skip" any part of the boot to save time.

After the script returns, run the readiness check from Phase 8c (`screen -ls`, poll `PROVIDER1.log` for "listening on" or fatal patterns).

If the script fails to spawn `provider1`/`consumers` screens or `PROVIDER1.log` shows `FTL`/`panic`/`failed to load spec`/`provider verification` failures, STOP. Present the log to the user — this is a REGRESSION introduced by Phase 10's fixes.

### Step 10b.2 — Re-probe a deterministic minimal set

1. `GET_BLOCKNUM` parse directive — same call as Phase 8.
2. `chain-id` verification — call the verification method and confirm response matches the spec's `expected_value`.
3. **5 sampled read methods** — deterministically the first 5 non-stateful, non-subscription APIs (alphabetical by name) from the largest collection.

### Step 10b.3 — Compare classifications

For each of these 7 probes, compare the Phase 10b classification against the Phase 8 classification (from `METHOD_PROBE_REPORT.md`):

- If a probe was PASS in Phase 8 and is now FAIL or TIMEOUT → **REGRESSION**.
- If a probe was FAIL/WARN/TIMEOUT in Phase 8 and is now PASS → improvement (record but do not alert).
- All else → no change.

If any REGRESSION is detected: surface the diff to the user. Show what was probed, what changed, and which fix from Phase 10 likely caused it. STOP. Do not proceed to Phase 11.

If no regressions, tear down the provider (Step 8f) and continue.

## Phase 11 — Final reviewer (clean context)

**Do NOT use `isolation: "worktree"`** — same reason as Phase 9. The candidate spec is uncommitted, so a worktree reviewer would see stale HEAD state. Fresh-subagent-context alone provides the anchoring isolation we need.

Before invoking the final reviewer, archive prior reports so the reviewer's `/review-spec` skill (Phase 1 of which scans `specs/docs/<CHAIN_NAME>/`) does not pick them up as anchoring. Also remove any stale `SPEC_REVIEW_GAPS.md` (without the `_parallel_N` suffix) that might be lingering:

```bash
mkdir -p specs/docs/<chain>/_archive
mv specs/docs/<chain>/SPEC_REVIEW_GAPS_parallel_*.md specs/docs/<chain>/_archive/ 2>/dev/null || true
mv specs/docs/<chain>/SPEC_REVIEW_FIXES_*.md specs/docs/<chain>/_archive/ 2>/dev/null || true
rm -f specs/docs/<chain>/SPEC_REVIEW_GAPS.md
```

Dispatch ONE Agent subagent with `subagent_type: general-purpose` (no `isolation` parameter). The prompt:

> You are reviewing a Lava blockchain spec — final pass after fixes were applied.
>
> Run the `/review-spec` skill on `specs/testnet-2/specs/<chain>.json`. Pass through `$ARGUMENTS[1]` and `$ARGUMENTS[2]`.
>
> Before running `/review-spec`, read `specs/docs/<chain>/METHOD_PROBE_REPORT.md` if it exists.
>
> `/review-spec` writes its report to `specs/docs/<chain>/SPEC_REVIEW_GAPS.md`. After it returns, rename to a final-pass-specific path:
>
> ```bash
> mv specs/docs/<chain>/SPEC_REVIEW_GAPS.md specs/docs/<chain>/SPEC_REVIEW_GAPS_final.md
> ```
>
> Return:
> 1. The FULL contents of `specs/docs/<chain>/SPEC_REVIEW_GAPS_final.md` as the body of your response.
> 2. On the LAST line of your response, print exactly: `TALLY: CRITICAL=<X> MEDIUM=<Y> MINOR=<Z>` with integer counts.

**Sanity check (same as Phase 9):** if the reviewer reports CRITICAL findings whose `evidence_line_number` exceeds the actual line count of `specs/testnet-2/specs/<chain>.json`, the reviewer reviewed stale state. Re-dispatch once.

Outcomes:
- TALLY shows `CRITICAL=0 MEDIUM=0` → **APPROVED**. Proceed to Phase 12.
- TALLY shows remaining CRITICAL or MEDIUM gaps → **CHANGES REQUESTED**. Present the report to the user. STOP — do not loop. (Avoids the `/review-and-fix-spec` "max-loops-exit-without-converging" failure mode.) Skip Phase 12.

## Phase 12 — Final summary checklist (printed to user; no auto-action)

Print the checklist below to the user verbatim, with each item annotated:

- `✓` — verified by this run; cite the phase that produced the evidence
- `~` — partially verified; one-line note on what's not covered
- `☐` — user to handle manually (out of skill scope; surfaced as a reminder)

If a phase was skipped (e.g., Phase 8 skipped because user didn't supply node URLs), downgrade the corresponding items from `✓`/`~` to `☐` with a "(phase N skipped)" note.

```text
#### File Validation
- ✓ JSON syntax valid                                    (Phase 7 ran `jq` autonomously)
- ✓ All required fields present                          (Phase 6 completeness checklist)
- ✓ No duplicate API names within any collection         (Phase 6)
- ✓ Proper indentation/formatting                        (jq-formatted output)

#### Configuration Verification
- ✓ Network parameters calculated per formulas           (Phase 4 calculations table)
- ~ All APIs tested and working                          (Phase 8 probe — see specs/docs/<chain>/METHOD_PROBE_REPORT.md; stateful methods skipped)
- ~ Block parsing validated for each API                 (Phase 8 existence-tested; full parse validation requires production traffic)
- ✓ Verifications pass on live nodes                     (Phase 6 chain-id curl + Phase 8 multi-node probe)
- ☐ Compute units benchmarked under expected load        (user to measure)
- ☐ Economic parameters reasonable (min_stake_provider, shares)  (user judgment)

#### Documentation (out of skill scope — manual reminder)
- ☐ SPEC_IMPLEMENTATION.md created
- ☐ API_REFERENCE.md created
- ☐ TESTING_GUIDE.md created
- ☐ QUICK_START.md created

#### Testnet vs Mainnet
- ✓ Mainnet spec complete                                (Phase 7 wrote both entries to single file)
- ✓ Testnet spec inherits correctly                      (single-file pattern; Phase 11 reviewer confirmed)
- ✓ Chain IDs verified for mainnet AND testnet           (Phase 6 dual curl + Phase 8 probe)
- ~ Both tested on respective networks                   (Phase 8 probed all node URLs provided per variant)

#### Governance Prep (out of skill scope — manual reminder)
- ☐ Proposal JSON formatted correctly                    (the file produced has the proposal wrapper; user verifies title/description)
- ☐ Proposal description written
- ☐ Deposit amount confirmed                             (default "10001000ulava" written; user confirms)
- ☐ Community feedback gathered (if applicable)
```

After printing, terminate the skill. The user takes it from here (manual git operations, governance flow if applicable).

## Out of scope

- Writing to `specs/mainnet-1/specs/` or `specs/testnet-1/specs/` — testnet-2 only
- Creating `specs/docs/<chain>/` documentation files beyond the probe and review reports the skill emits during its own run
- Creating governance proposal JSONs or `PROPOSAL_DESCRIPTION.md`
- Any git operations: `git add`, `git commit`, `git push`, `git checkout`, `glab mr create`. User handles all git manually.

If the user asks for any of these, surface the limitation and confirm scope before continuing.
