package lavasession

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/lavanet/lava/v5/utils"
)

// errNotWhitelistDocument marks a file that is valid JSON but is not a whitelist (it omits the
// "providers" key). During a multi-file load these are skipped so a shared/mixed repo of unrelated
// configs doesn't break the load. It is deliberately distinct from a malformed-JSON error, which
// is treated as a hard failure (a corrupt file must not silently drop its chains from the snapshot).
var errNotWhitelistDocument = errors.New(`not a provider whitelist document (missing "providers" field)`)

// providerWhitelistJSON is the JSON schema of the consumer provider whitelist, as fetched from
// GitHub or a local file. It is intentionally unrelated to the spec schema
// (specutils.SpecAddProposalJSON): the whitelist is a flat list of providers and the chains
// each provider is allowed to serve.
//
//	{
//	  "providers": [
//	    { "address": "lava@1abc...", "chains": ["ETH1", "LAV1"] },
//	    { "address": "lava@1def...", "chains": ["NEAR"] }
//	  ]
//	}
//
// Providers is a pointer so we can distinguish a document that omits the "providers" key (not a
// whitelist file at all -> rejected) from one that sets it to an empty array (an intentional
// "allow nobody" whitelist -> accepted).
type providerWhitelistJSON struct {
	Providers *[]providerWhitelistEntry `json:"providers"`
}

type providerWhitelistEntry struct {
	Address string   `json:"address"`
	Chains  []string `json:"chains"`
}

// whitelistData is the immutable, lookup-optimized index built from the parsed JSON:
// chainID -> set of allowed provider addresses.
type whitelistData struct {
	byChain map[string]map[string]struct{}
}

func (wd *whitelistData) isAllowed(chainID, providerAddr string) bool {
	addrs, ok := wd.byChain[chainID]
	if !ok {
		return false
	}
	_, ok = addrs[providerAddr]
	return ok
}

// pairCount returns the number of distinct (chain, provider) pairs, used for logging.
func (wd *whitelistData) pairCount() int {
	count := 0
	for _, addrs := range wd.byChain {
		count += len(addrs)
	}
	return count
}

// ProviderWhitelist is a thread-safe, hourly-refreshed allowlist that restricts which providers
// the consumer is willing to relay to, per chain. A single instance is shared across all
// per-chain ConsumerSessionManagers; each manager queries it with its own chainID.
//
// Semantics:
//   - Until the first successful load it is "not loaded": IsAllowed returns true (passthrough), so
//     a startup fetch failure keeps the consumer relaying to everyone rather than going dark.
//   - Once loaded, IsAllowed returns true only for (chainID, providerAddr) pairs present in the
//     list. A loaded-but-empty list therefore allows nobody (intentional whitelist semantics). A
//     later refresh failure does NOT clear the snapshot, so the last-known-good list stays in place.
//
// The active snapshot is held in an atomic.Pointer so the read hot path (IsAllowed, called
// per-candidate-provider per-relay) takes no locks, and the hourly refresh swaps it atomically.
type ProviderWhitelist struct {
	data atomic.Pointer[whitelistData]
}

// NewProviderWhitelist returns a whitelist in the "not loaded" (passthrough) state. Until the first
// successful load the consumer relays to everyone; once loaded a refresh failure keeps the
// last-known-good snapshot.
func NewProviderWhitelist() *ProviderWhitelist {
	return &ProviderWhitelist{}
}

// Enabled reports whether a whitelist has been successfully loaded at least once.
func (pw *ProviderWhitelist) Enabled() bool {
	return pw.data.Load() != nil
}

// snapshot returns the current whitelist index, or nil if no whitelist has loaded yet. Callers
// filtering a list should snapshot once and reuse it across elements rather than calling
// IsAllowed per element.
func (pw *ProviderWhitelist) snapshot() *whitelistData {
	return pw.data.Load()
}

// IsAllowed reports whether the consumer may relay to providerAddr for chainID. It returns true
// (passthrough) when no whitelist has been loaded yet, so a startup fetch failure keeps the
// consumer relaying to everyone rather than going dark.
func (pw *ProviderWhitelist) IsAllowed(chainID, providerAddr string) bool {
	data := pw.data.Load()
	if data == nil {
		return true // not loaded yet -> passthrough (allow all until the first successful load)
	}
	return data.isAllowed(chainID, providerAddr)
}

// UpdateFromBytes parses a single whitelist JSON document and atomically replaces the active
// snapshot. On error the previous snapshot is left intact.
func (pw *ProviderWhitelist) UpdateFromBytes(content []byte) error {
	index, err := parseWhitelist(content)
	if err != nil {
		return err
	}
	pw.store(index)
	return nil
}

// UpdateFromFiles parses each provided file as a whitelist document and atomically replaces the
// active snapshot with the union of their entries. Files that are valid JSON but are not whitelist
// documents (no "providers" key) are skipped with a warning, so a shared/mixed repo does not break
// the load. Any other parse failure (malformed JSON) is treated as a hard error: the previous
// snapshot is left intact and an error is returned, so a corrupt file can't silently drop its
// chains from the snapshot. If no file yields a valid whitelist, the previous snapshot is likewise
// left intact and an error is returned.
func (pw *ProviderWhitelist) UpdateFromFiles(files map[string][]byte) error {
	merged := map[string]map[string]struct{}{}
	parsedAny := false
	for fileURL, content := range files {
		index, err := parseWhitelist(content)
		if err != nil {
			if errors.Is(err, errNotWhitelistDocument) {
				utils.LavaFormatWarning("skipping non-whitelist file in provider whitelist source", err, utils.LogAttr("file", fileURL))
				continue
			}
			// Malformed whitelist file: fail the whole refresh and keep the last-known-good snapshot
			// rather than swapping in a partial union that silently omits this file's chains.
			return fmt.Errorf("malformed provider whitelist file %q: %w", fileURL, err)
		}
		parsedAny = true
		for chainID, addrs := range index.byChain {
			if merged[chainID] == nil {
				merged[chainID] = map[string]struct{}{}
			}
			for addr := range addrs {
				merged[chainID][addr] = struct{}{}
			}
		}
	}
	if !parsedAny {
		return fmt.Errorf("no valid provider whitelist files found (out of %d fetched)", len(files))
	}
	pw.store(&whitelistData{byChain: merged})
	return nil
}

// store atomically swaps in a new snapshot and logs the result. An empty (but loaded) whitelist
// is logged loudly because it causes the consumer to relay to no providers.
func (pw *ProviderWhitelist) store(data *whitelistData) {
	pw.data.Store(data)
	pairs := data.pairCount()
	if pairs == 0 {
		utils.LavaFormatWarning("provider whitelist loaded but EMPTY - the consumer will relay to NO providers", nil)
		return
	}
	utils.LavaFormatInfo("provider whitelist loaded",
		utils.LogAttr("chains", len(data.byChain)),
		utils.LogAttr("allowed_provider_chain_pairs", pairs),
	)
}

// parseWhitelist parses a single whitelist JSON document into a lookup index. It returns an error
// for malformed JSON or for a document that omits the "providers" key (i.e. is not a whitelist).
func parseWhitelist(content []byte) (*whitelistData, error) {
	var doc providerWhitelistJSON
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse provider whitelist JSON: %w", err)
	}
	if doc.Providers == nil {
		return nil, errNotWhitelistDocument
	}

	byChain := map[string]map[string]struct{}{}
	for _, entry := range *doc.Providers {
		if entry.Address == "" {
			continue
		}
		for _, chainID := range entry.Chains {
			if chainID == "" {
				continue
			}
			if byChain[chainID] == nil {
				byChain[chainID] = map[string]struct{}{}
			}
			byChain[chainID][entry.Address] = struct{}{}
		}
	}
	return &whitelistData{byChain: byChain}, nil
}
