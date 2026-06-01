package rpcconsumer

import (
	"context"
	"os"
	"time"

	"github.com/lavanet/lava/v5/protocol/common"
	"github.com/lavanet/lava/v5/protocol/lavasession"
	"github.com/lavanet/lava/v5/utils"
	"github.com/lavanet/lava/v5/utils/specfetcher"
)

// providerWhitelistRetryInterval is the short interval used to retry the initial whitelist load
// until it first succeeds. This avoids a long fail-open window: if the source is briefly
// unreachable at startup, we keep retrying quickly instead of waiting a full refresh interval.
const providerWhitelistRetryInterval = 30 * time.Second

// ProviderWhitelistFetcher periodically loads the consumer provider whitelist from its source and
// applies it to the shared *lavasession.ProviderWhitelist. The source is either:
//   - a GitHub/GitLab directory URL, fetched with the exact same machinery as specs
//     (specfetcher), authenticated with the existing github/gitlab tokens; or
//   - a local JSON file path.
//
// A single fetcher serves all chains (the whitelist is global; each per-chain session manager
// queries it with its own ChainID). It is only constructed/started when a source is configured,
// so when the flag is empty no refresh loop runs at all.
type ProviderWhitelistFetcher struct {
	source          string
	githubToken     string
	gitlabToken     string
	refreshInterval time.Duration
	target          *lavasession.ProviderWhitelist
}

func NewProviderWhitelistFetcher(source, githubToken, gitlabToken string, refreshInterval time.Duration, target *lavasession.ProviderWhitelist) *ProviderWhitelistFetcher {
	return &ProviderWhitelistFetcher{
		source:          source,
		githubToken:     githubToken,
		gitlabToken:     gitlabToken,
		refreshInterval: refreshInterval,
		target:          target,
	}
}

// Start performs an initial load (retrying on a short interval until the first success) and then
// refreshes on refreshInterval until ctx is cancelled. Intended to run in its own goroutine.
func (f *ProviderWhitelistFetcher) Start(ctx context.Context) {
	if f.source == "" || f.target == nil {
		return
	}
	// Defensive floor: guarantees a valid ticker interval even if this fetcher is constructed
	// directly with a zero/negative interval (the cobra wiring already applies the same default).
	if f.refreshInterval <= 0 {
		f.refreshInterval = common.DefaultProvidersWhitelistRefreshInterval
	}

	utils.LavaFormatInfo("starting provider whitelist fetcher",
		utils.LogAttr("source", f.source),
		utils.LogAttr("refresh_interval", f.refreshInterval),
	)

	// Initial load: retry quickly until the first success so the consumer doesn't relay in
	// passthrough for a whole refresh interval if the source is briefly unavailable at startup.
	if !f.loadOnce(ctx) {
		retryInterval := providerWhitelistRetryInterval
		if f.refreshInterval > 0 && f.refreshInterval < retryInterval {
			retryInterval = f.refreshInterval
		}
		retry := time.NewTicker(retryInterval)
		defer retry.Stop()
	initialLoad:
		for {
			select {
			case <-ctx.Done():
				return
			case <-retry.C:
				if f.loadOnce(ctx) {
					break initialLoad
				}
			}
		}
	}

	ticker := time.NewTicker(f.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.loadOnce(ctx)
		}
	}
}

// loadOnce fetches the whitelist from its source and applies it to the target, returning true on
// success. On failure the previous (last-known-good) whitelist is left intact; the consumer keeps
// using it, and if nothing has loaded yet it stays in passthrough.
func (f *ProviderWhitelistFetcher) loadOnce(ctx context.Context) bool {
	if specfetcher.IsRemoteRepoURL(f.source) {
		return f.loadFromRemote(ctx)
	}
	return f.loadFromFile()
}

func (f *ProviderWhitelistFetcher) loadFromRemote(ctx context.Context) bool {
	// Pick the token by provider, mirroring how specs are loaded (statetracker.loadAllSpecsFromRemoteRepo).
	token := ""
	if specfetcher.IsGitHubURL(f.source) {
		token = f.githubToken
	} else if specfetcher.IsGitLabURL(f.source) {
		token = f.gitlabToken
	}

	files, err := specfetcher.FetchAllFilesFromRemote(ctx, f.source, token)
	if err != nil {
		utils.LavaFormatError("failed fetching provider whitelist from remote, keeping previous list", err, utils.LogAttr("source", f.source))
		return false
	}
	if err := f.target.UpdateFromFiles(files); err != nil {
		utils.LavaFormatError("failed parsing provider whitelist from remote, keeping previous list", err, utils.LogAttr("source", f.source))
		return false
	}
	return true
}

func (f *ProviderWhitelistFetcher) loadFromFile() bool {
	content, err := os.ReadFile(f.source)
	if err != nil {
		utils.LavaFormatError("failed reading provider whitelist file, keeping previous list", err, utils.LogAttr("source", f.source))
		return false
	}
	if err := f.target.UpdateFromBytes(content); err != nil {
		utils.LavaFormatError("failed parsing provider whitelist file, keeping previous list", err, utils.LogAttr("source", f.source))
		return false
	}
	return true
}
