package lavasession

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleWhitelistJSON = `{
  "providers": [
    { "address": "provider0", "chains": ["ETH1", "LAV1"] },
    { "address": "provider1", "chains": ["ETH1"] },
    { "address": "provider2", "chains": ["NEAR"] }
  ]
}`

func TestProviderWhitelist_NotLoadedIsPassthrough(t *testing.T) {
	pw := NewProviderWhitelist()
	require.False(t, pw.Enabled())
	// Until a list loads, everything is allowed (current relay behavior).
	require.True(t, pw.IsAllowed("ETH1", "anyProvider"))
	require.True(t, pw.IsAllowed("anyChain", "anyProvider"))
}

func TestProviderWhitelist_IsAllowedHitAndMiss(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(sampleWhitelistJSON)))
	require.True(t, pw.Enabled())

	// Hits: (provider, chain) pairs present in the list.
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.True(t, pw.IsAllowed("LAV1", "provider0"))
	require.True(t, pw.IsAllowed("ETH1", "provider1"))
	require.True(t, pw.IsAllowed("NEAR", "provider2"))

	// Misses: provider present but not on that chain.
	require.False(t, pw.IsAllowed("LAV1", "provider1")) // provider1 is only on ETH1
	require.False(t, pw.IsAllowed("NEAR", "provider0"))
	// Miss: provider not in the list at all.
	require.False(t, pw.IsAllowed("ETH1", "providerX"))
	// Miss: chain not in the list at all.
	require.False(t, pw.IsAllowed("BTC", "provider0"))
}

func TestProviderWhitelist_PerChainIsolation(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(sampleWhitelistJSON)))
	// The same provider is allowed on one chain and denied on another.
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.False(t, pw.IsAllowed("NEAR", "provider0"))
}

func TestProviderWhitelist_EmptyListAllowsNobody(t *testing.T) {
	pw := NewProviderWhitelist()
	// A loaded-but-empty whitelist is "allow nobody" (intentional whitelist semantics).
	require.NoError(t, pw.UpdateFromBytes([]byte(`{"providers":[]}`)))
	require.True(t, pw.Enabled())
	require.False(t, pw.IsAllowed("ETH1", "provider0"))
}

func TestProviderWhitelist_MalformedKeepsPrevious(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(sampleWhitelistJSON)))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))

	// A malformed update must error and leave the last-known-good snapshot intact.
	require.Error(t, pw.UpdateFromBytes([]byte(`{ not valid json `)))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
}

func TestProviderWhitelist_MissingProvidersFieldRejected(t *testing.T) {
	pw := NewProviderWhitelist()
	// A document without a "providers" key is not a whitelist (e.g. a spec file) and is rejected,
	// rather than being treated as an empty (allow-nobody) whitelist.
	require.Error(t, pw.UpdateFromBytes([]byte(`{"proposal":{"specs":[]}}`)))
	require.False(t, pw.Enabled())
}

func TestProviderWhitelist_AtomicSwapReplacesList(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(`{"providers":[{"address":"provider0","chains":["ETH1"]}]}`)))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.False(t, pw.IsAllowed("ETH1", "provider1"))

	require.NoError(t, pw.UpdateFromBytes([]byte(`{"providers":[{"address":"provider1","chains":["ETH1"]}]}`)))
	require.False(t, pw.IsAllowed("ETH1", "provider0"))
	require.True(t, pw.IsAllowed("ETH1", "provider1"))
}

func TestProviderWhitelist_UpdateFromFilesUnions(t *testing.T) {
	pw := NewProviderWhitelist()
	files := map[string][]byte{
		"a.json": []byte(`{"providers":[{"address":"provider0","chains":["ETH1"]}]}`),
		"b.json": []byte(`{"providers":[{"address":"provider1","chains":["ETH1","LAV1"]}]}`),
	}
	require.NoError(t, pw.UpdateFromFiles(files))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.True(t, pw.IsAllowed("ETH1", "provider1"))
	require.True(t, pw.IsAllowed("LAV1", "provider1"))
	require.False(t, pw.IsAllowed("LAV1", "provider0"))
}

func TestProviderWhitelist_UpdateFromFilesSkipsNonWhitelist(t *testing.T) {
	pw := NewProviderWhitelist()
	files := map[string][]byte{
		"whitelist.json": []byte(`{"providers":[{"address":"provider0","chains":["ETH1"]}]}`),
		"spec.json":      []byte(`{"proposal":{"specs":[{"index":"ETH1"}]}}`), // valid JSON, no "providers" -> skipped
	}
	// Succeeds because at least one file is a valid whitelist; files that are valid JSON but aren't
	// whitelist documents are skipped (mixed-repo support).
	require.NoError(t, pw.UpdateFromFiles(files))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
}

// TestProviderWhitelist_UpdateFromFilesMalformedKeepsPrevious pins the all-or-nothing contract for
// a corrupt file: a file that fetched OK but is malformed JSON fails the whole refresh and leaves
// the last-known-good snapshot intact, instead of swapping in a partial union that silently drops
// the good file's chains. (A malformed JSON file is distinct from a valid-JSON non-whitelist file,
// which is still skipped — see TestProviderWhitelist_UpdateFromFilesSkipsNonWhitelist.)
func TestProviderWhitelist_UpdateFromFilesMalformedKeepsPrevious(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(sampleWhitelistJSON)))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))

	files := map[string][]byte{
		"good.json":    []byte(`{"providers":[{"address":"providerX","chains":["NEAR"]}]}`),
		"corrupt.json": []byte(`not json at all`), // malformed -> hard failure, no swap
	}
	require.Error(t, pw.UpdateFromFiles(files))
	// The previous snapshot is untouched: the good file's NEAR provider was NOT swapped in, and the
	// original ETH1 provider is still allowed.
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.False(t, pw.IsAllowed("NEAR", "providerX"))
}

func TestProviderWhitelist_UpdateFromFilesAllInvalidKeepsPrevious(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(sampleWhitelistJSON)))

	files := map[string][]byte{
		"spec.json":    []byte(`{"proposal":{"specs":[]}}`),
		"garbage.json": []byte(`nope`),
	}
	// No file is a valid whitelist -> error, and the previous snapshot is preserved.
	require.Error(t, pw.UpdateFromFiles(files))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
}

func TestProviderWhitelist_IgnoresEmptyAddressAndChain(t *testing.T) {
	pw := NewProviderWhitelist()
	require.NoError(t, pw.UpdateFromBytes([]byte(`{"providers":[
		{"address":"","chains":["ETH1"]},
		{"address":"provider0","chains":["",""]},
		{"address":"provider1","chains":["ETH1",""]}
	]}`)))
	// Entry with empty address contributes nothing; empty chain ids are skipped.
	require.True(t, pw.Enabled())
	require.True(t, pw.IsAllowed("ETH1", "provider1"))
	require.False(t, pw.IsAllowed("ETH1", "provider0")) // only had empty chains
	require.False(t, pw.IsAllowed("ETH1", ""))
}
