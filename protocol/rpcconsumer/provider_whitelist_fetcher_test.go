package rpcconsumer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lavanet/lava/v5/protocol/lavasession"
	"github.com/stretchr/testify/require"
)

func writeTempWhitelist(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "whitelist.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestProviderWhitelistFetcher_LoadFromLocalFile(t *testing.T) {
	path := writeTempWhitelist(t, `{"providers":[{"address":"provider0","chains":["ETH1","LAV1"]}]}`)
	pw := lavasession.NewProviderWhitelist()
	fetcher := NewProviderWhitelistFetcher(path, "", "", time.Hour, pw)

	require.True(t, fetcher.loadOnce(context.Background()))
	require.True(t, pw.Enabled())
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
	require.True(t, pw.IsAllowed("LAV1", "provider0"))
	require.False(t, pw.IsAllowed("ETH1", "provider1"))
}

func TestProviderWhitelistFetcher_MalformedRefreshKeepsPrevious(t *testing.T) {
	path := writeTempWhitelist(t, `{"providers":[{"address":"provider0","chains":["ETH1"]}]}`)
	pw := lavasession.NewProviderWhitelist()
	fetcher := NewProviderWhitelistFetcher(path, "", "", time.Hour, pw)

	require.True(t, fetcher.loadOnce(context.Background()))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))

	// Overwrite the source with malformed content; the refresh fails and keeps the last-good list.
	require.NoError(t, os.WriteFile(path, []byte(`{ not valid json `), 0o600))
	require.False(t, fetcher.loadOnce(context.Background()))
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
}

func TestProviderWhitelistFetcher_MissingFileKeepsPassthrough(t *testing.T) {
	pw := lavasession.NewProviderWhitelist()
	fetcher := NewProviderWhitelistFetcher(filepath.Join(t.TempDir(), "does-not-exist.json"), "", "", time.Hour, pw)

	require.False(t, fetcher.loadOnce(context.Background()))
	// Never loaded -> stays in passthrough.
	require.False(t, pw.Enabled())
	require.True(t, pw.IsAllowed("ETH1", "provider0"))
}
