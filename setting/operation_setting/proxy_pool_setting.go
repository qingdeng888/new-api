package operation_setting

import (
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ProxyPoolSetting holds the single global outbound proxy pool. Channels and
// individual multi-key entries bind to it with "pool" (random rotation) or
// "pool:<n>" (pinned member), instead of naming separate pools.
type ProxyPoolSetting struct {
	// One proxy per line; blank lines and #-comments are ignored. Entries
	// accept direct, http://, https://, socks5:// and socks5h:// including
	// user:password credentials.
	Proxies string `json:"proxies"`
}

var proxyPoolSetting = ProxyPoolSetting{}

func init() {
	config.GlobalConfig.Register("proxy_pool_setting", &proxyPoolSetting)
}

func GetProxyPoolSetting() *ProxyPoolSetting {
	return &proxyPoolSetting
}

// ProxyPoolEntries returns the configured proxy URLs in order.
func ProxyPoolEntries() []string {
	return SplitProxyPoolEntries(proxyPoolSetting.Proxies)
}

// SplitProxyPoolEntries parses a proxy list document, skipping blank lines
// and #-comments.
func SplitProxyPoolEntries(document string) []string {
	var entries []string
	for _, raw := range strings.Split(document, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, line)
	}
	return entries
}

// ValidateProxyPoolDocument rejects documents with a malformed entry and
// returns the entries when the whole document is valid.
func ValidateProxyPoolDocument(document string) ([]string, error) {
	entries := SplitProxyPoolEntries(document)
	for _, entry := range entries {
		if entry == "direct" {
			continue
		}
		if _, err := common.ParseProxyURLStrict(entry); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// Runtime health of the pool members, keyed by proxy URL. Cool-downs grow
// from 15s and double up to 120s; a success clears the state.
const (
	ProxyCooldownBaseSeconds int64 = 15
	ProxyCooldownMaxSeconds  int64 = 120
)

type proxyHealthState struct {
	until  int64
	streak int
}

var (
	proxyPoolHealthLock sync.Mutex
	proxyPoolHealth     = make(map[string]proxyHealthState)
)

// ProxyCooldownSeconds returns the cool-down for a member whose consecutive
// failure streak is `streak` (1-based).
func ProxyCooldownSeconds(streak int) int64 {
	if streak < 1 {
		streak = 1
	}
	seconds := ProxyCooldownBaseSeconds
	for range streak - 1 {
		seconds *= 2
		if seconds >= ProxyCooldownMaxSeconds {
			return ProxyCooldownMaxSeconds
		}
	}
	return seconds
}

func IsProxyCooling(proxyURL string, now int64) bool {
	state, ok := proxyPoolHealth[proxyURL]
	return ok && state.until > now
}

// MarkProxyTransportResult records the outcome of a request made through a
// pool member so random rotation can skip failing exits.
func MarkProxyTransportResult(proxyURL string, success bool) {
	if proxyURL == "" {
		return
	}
	proxyPoolHealthLock.Lock()
	defer proxyPoolHealthLock.Unlock()
	if success {
		delete(proxyPoolHealth, proxyURL)
		return
	}
	state := proxyPoolHealth[proxyURL]
	state.streak++
	state.until = common.GetTimestamp() + ProxyCooldownSeconds(state.streak)
	proxyPoolHealth[proxyURL] = state
}

// PickProxyPoolMember resolves a "pool" or "pool:<n>" binding to one proxy
// URL from the global list. Random mode skips members in cool-down and
// transfers traffic to the rest; a fully cooled pool falls back to the
// member that expires first. Pinned mode prefers its member and transfers
// to another healthy one while it cools down, returning once it recovers.
func PickProxyPoolMember(binding string) (string, bool) {
	if binding != "pool" && !strings.HasPrefix(binding, "pool:") {
		return "", false
	}
	entries := ProxyPoolEntries()
	if len(entries) == 0 {
		return "", false
	}

	proxyPoolHealthLock.Lock()
	defer proxyPoolHealthLock.Unlock()
	now := common.GetTimestamp()
	healthy := func(idx int) bool {
		return !IsProxyCooling(entries[idx], now)
	}
	earliest := func() int {
		best, bestUntil := 0, int64(-1)
		for i, entry := range entries {
			until := int64(0)
			if state, ok := proxyPoolHealth[entry]; ok {
				until = state.until
			}
			if bestUntil < 0 || until < bestUntil {
				bestUntil, best = until, i
			}
		}
		return best
	}

	if binding == "pool" {
		var healthyIdx []int
		for i := range entries {
			if healthy(i) {
				healthyIdx = append(healthyIdx, i)
			}
		}
		if len(healthyIdx) > 0 {
			return entries[healthyIdx[rand.Intn(len(healthyIdx))]], true
		}
		return entries[earliest()], true
	}

	pinned := strings.TrimPrefix(binding, "pool:")
	index, err := strconv.Atoi(pinned)
	if err != nil || index < 0 || index >= len(entries) {
		return "", false
	}
	if healthy(index) {
		return entries[index], true
	}
	for i := 1; i < len(entries); i++ {
		candidate := (index + i) % len(entries)
		if healthy(candidate) {
			return entries[candidate], true
		}
	}
	return entries[earliest()], true
}
