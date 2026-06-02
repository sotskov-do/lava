# Consumer Provider Whitelist

## Goal

Give an `rpcconsumer` operator a way to **restrict which providers the consumer relays to**,
on top of the on-chain pairing, using an externally-managed list of `(provider, chain)` pairs.
The list is refreshed periodically (hourly by default) from **GitHub/GitLab or a local file**,
so it can be updated without restarting the consumer.

> "Build a list of providers and the chains they support, fetched every hour from GitHub or a
> local file, and only relay to providers in that list. If the list does not exist, keep the
> current relay behavior."

This is a **client-side** filter. It is unrelated to the on-chain
`x/pairing/keeper/filters/selected_providers_filter.go` (EXCLUSIVE/MIXED policy filter), which
is a consensus-level mechanism.

## TL;DR

- Set `--providers-whitelist-config` to a JSON file path or a GitHub/GitLab directory URL.
- When the list is configured **and successfully loaded**, the consumer relays **only** to
  whitelisted `(provider address, chain)` pairs.
- When the flag is **empty** (default) **or no list has loaded yet**, the consumer keeps its
  **current behavior** (relay to any paired provider) and **no refresh loop runs**.
- The list is re-fetched every `--providers-whitelist-refresh-interval` (default `1h`).

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--providers-whitelist-config` | `""` (disabled) | JSON file path **or** a GitHub/GitLab directory URL. Empty keeps current relay behavior. |
| `--providers-whitelist-refresh-interval` | `1h` | How often the list is re-fetched. Only used when a source is configured. |
| `--providers-whitelist-token` | `""` | Dedicated access token for a **remote** source in a different repo than the specs. Falls back to `--github-token` / `--gitlab-token` (by provider) when empty. |

The flags are defined in `protocol/common/cobra_common.go` and registered for `rpcconsumer` in
`protocol/rpcconsumer/rpcconsumer.go`.

### Example: local file

```bash
rpcconsumer rpcconsumer.yml \
  --providers-whitelist-config ./provider-whitelist.json \
  --providers-whitelist-refresh-interval 1h
```

### Example: GitHub (fetched exactly like specs)

```bash
rpcconsumer rpcconsumer.yml \
  --providers-whitelist-config https://github.com/{owner}/{repo}/tree/{branch}/{path} \
  --providers-whitelist-token ghp_xxxxxxxx        # optional; else falls back to --github-token
```

The GitHub/GitLab URL format is identical in shape to the spec source
(`--use-static-spec`): a `/tree/{branch}/{path}` **directory** URL.

## List format

The list is a JSON document describing providers and the chains each is allowed to serve:

```json
{
  "providers": [
    { "address": "lava@1abc...", "chains": ["ETH1", "LAV1"] },
    { "address": "lava@1def...", "chains": ["NEAR"] }
  ]
}
```

- `address` — the provider's **on-chain bech32 address** (`lava@1...`). It is matched **exactly**
  (case- and whitespace-sensitive); it must match the address as staked on-chain.
- `chains` — the spec chain IDs (e.g. `ETH1`, `LAV1`) this provider is allowed to serve.

A GitHub/GitLab directory may contain **one or more** `.json` files; their `providers` arrays are
**unioned** (the same way specs are split across files). Files that don't conform to this schema
(for example a spec file in a shared directory) are skipped with a warning. A local source is a
single JSON file.

The schema and parsing live in `protocol/lavasession/provider_whitelist.go`.

## How fetching works

Remote fetching reuses the **exact same machinery as specs** — URL parsing, GitHub/GitLab
directory listing, authenticated raw-file download, HTTP client, and timeouts — via
`specfetcher.FetchAllFilesFromRemote` (`utils/specfetcher/api.go`). The only thing that differs
from specs is the terminal JSON parse (whitelist schema, not `types.Spec`).

The background refresher is `ProviderWhitelistFetcher`
(`protocol/rpcconsumer/provider_whitelist_fetcher.go`):

1. On startup it attempts an initial load, **retrying on a short interval (30s) until the first
   success** — this avoids a long "fail-open" window if the source is briefly unreachable.
2. After the first successful load, it re-fetches every refresh interval (default `1h`).
3. The active list is held in an `atomic.Pointer`, so the read path (`IsAllowed`, called per
   candidate provider per relay) is lock-free and the refresh swaps the list atomically.

## How filtering works

Each per-chain `ConsumerSessionManager` (CSM) is injected with the shared whitelist and queries it
with **its own** `ChainID` (`SetProviderWhitelist`, `consumer_session_manager.go`). A non-whitelisted
provider is filtered out of **every** provider-selection path:

| Selection path | Where it is guarded |
|----------------|---------------------|
| Optimizer (normal selection) | `validAddresses` is filtered in `getValidProviderAddresses` before the optimizer reads it |
| Header-selected provider | same `validAddresses` filter (the header shortcut gates on membership) |
| Sticky session | same `validAddresses` filter (the sticky shortcut gates on membership) |
| Blocked-provider recovery | per-candidate guard in `tryGetConsumerSessionWithProviderFromBlockedProviderList` |

The blocked-recovery guard matters specifically because the list refreshes on its own clock: a
provider that was whitelisted when it got blocked can be **de-listed** before it is recovered, and
must not be relayed to.

Filtering is applied at the **selection call site**, not inside the addon-address calculation that
`validatePairingListNotEmpty` drives. This way, when the whitelist intersection is empty, the
consumer returns a clean "no available providers" error (`PairingListEmptyError`) instead of
triggering repeated `validAddresses` resets.

## Behavior and failure modes

| Situation | Behavior |
|-----------|----------|
| Flag empty (not configured) | Passthrough — relay to any paired provider (current behavior). No refresh loop runs. |
| Configured, first fetch fails at startup | Passthrough, with **short-interval retry** until the first success. Logged loudly. |
| Configured, loads as empty `{"providers":[]}` | Loaded and **enabled** → relay to **nobody** for every chain. Logged loudly (a footgun, but correct whitelist semantics). |
| Configured, a later refresh fails or returns malformed JSON | Keep the **last-known-good** list; log the error. |
| Whitelist excludes every paired provider for a chain | Relays for that chain fail cleanly with `PairingListEmptyError`. |

## Scope

The feature is wired into **`rpcconsumer` only**: the whitelist is injected into each per-chain
`ConsumerSessionManager` at consumer startup (`SetProviderWhitelist`). A `ConsumerSessionManager`
that is never handed a whitelist keeps its current behavior (the nil-instance passthrough case
above).
