package operation_setting

import (
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ProxyPoolEntry is one outbound proxy in the global pool.
type ProxyPoolEntry struct {
	URL     string `json:"url"`
	Note    string `json:"note,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ProxyPoolSetting holds the single global outbound proxy pool. Channels and
// individual keys bind to it with "pool" (random rotation) or
// "pool:url:<url>" (a specific entry), instead of naming separate pools.
type ProxyPoolSetting struct {
	// JSON array of ProxyPoolEntry. Legacy plain-text documents (one proxy per
	// line) are still accepted and upgraded on the next save.
	Proxies string `json:"proxies"`
}

// ProxyPoolBindingPinnedPrefix marks a binding that pins one pool entry by URL.
const ProxyPoolBindingPinnedPrefix = "pool:url:"

var proxyPoolSetting = ProxyPoolSetting{}

func init() {
	config.GlobalConfig.Register("proxy_pool_setting", &proxyPoolSetting)
}

func GetProxyPoolSetting() *ProxyPoolSetting {
	return &proxyPoolSetting
}

// Parsed entries are cached because pool bindings resolve on every relayed
// request; the cache rebuilds lazily whenever the raw document changes.
var proxyPoolDocumentCache struct {
	sync.RWMutex
	raw     string
	entries []ProxyPoolEntry
}

func currentProxyPoolEntries() []ProxyPoolEntry {
	proxyPoolDocumentCache.RLock()
	if proxyPoolDocumentCache.raw == proxyPoolSetting.Proxies {
		defer proxyPoolDocumentCache.RUnlock()
		return proxyPoolDocumentCache.entries
	}
	proxyPoolDocumentCache.RUnlock()

	proxyPoolDocumentCache.Lock()
	defer proxyPoolDocumentCache.Unlock()
	if proxyPoolDocumentCache.raw != proxyPoolSetting.Proxies {
		proxyPoolDocumentCache.raw = proxyPoolSetting.Proxies
		proxyPoolDocumentCache.entries = ParseProxyPoolDocument(proxyPoolSetting.Proxies)
	}
	return proxyPoolDocumentCache.entries
}

// ParseProxyPoolDocument parses the stored document into entries. It accepts
// the JSON array form and the legacy line-based form.
func ParseProxyPoolDocument(document string) []ProxyPoolEntry {
	trimmed := strings.TrimSpace(document)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var entries []ProxyPoolEntry
		if err := common.UnmarshalJsonStr(trimmed, &entries); err == nil {
			return entries
		}
	}
	var entries []ProxyPoolEntry
	for _, raw := range strings.Split(document, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, ProxyPoolEntry{URL: line, Enabled: true})
	}
	return entries
}

// SerializeProxyPoolDocument renders entries as the stored JSON document.
func SerializeProxyPoolDocument(entries []ProxyPoolEntry) string {
	if len(entries) == 0 {
		return ""
	}
	encoded, err := common.Marshal(entries)
	if err != nil {
		common.SysLog(common.LogText("failed to serialize proxy pool: %v", err))
		return ""
	}
	return string(encoded)
}

// ValidateProxyPoolDocument rejects documents with a malformed entry.
func ValidateProxyPoolDocument(document string) ([]ProxyPoolEntry, error) {
	entries := ParseProxyPoolDocument(document)
	for _, entry := range entries {
		proxyURL := strings.TrimSpace(entry.URL)
		if proxyURL == "" {
			return nil, errInvalidProxyPoolEntry("")
		}
		if proxyURL == "direct" {
			continue
		}
		if _, err := common.ParseProxyURLStrict(proxyURL); err != nil {
			return nil, errInvalidProxyPoolEntry(proxyURL)
		}
	}
	return entries, nil
}

type errInvalidProxyPoolEntry string

func (e errInvalidProxyPoolEntry) Error() string {
	if string(e) == "" {
		return "proxy URL is required"
	}
	return "invalid proxy URL: " + string(e)
}

// ProxyPoolAllEntries returns every configured entry, including disabled ones.
func ProxyPoolAllEntries() []ProxyPoolEntry {
	return currentProxyPoolEntries()
}

// ProxyPoolEntries returns the URLs of the enabled entries in order.
func ProxyPoolEntries() []string {
	var urls []string
	for _, entry := range currentProxyPoolEntries() {
		if entry.Enabled && strings.TrimSpace(entry.URL) != "" {
			urls = append(urls, entry.URL)
		}
	}
	return urls
}

// FindProxyPoolEntry returns the entry with the given URL, if present.
func FindProxyPoolEntry(proxyURL string) (ProxyPoolEntry, bool) {
	target := strings.TrimSpace(proxyURL)
	for _, entry := range currentProxyPoolEntries() {
		if entry.URL == target {
			return entry, true
		}
	}
	return ProxyPoolEntry{}, false
}

// ProxyPoolHasURL reports whether the pool contains the URL in any state.
func ProxyPoolHasURL(proxyURL string) bool {
	_, ok := FindProxyPoolEntry(proxyURL)
	return ok
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
	proxyPoolHealthLock sync.RWMutex
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
// pool member so rotation can skip failing exits.
func MarkProxyTransportResult(proxyURL string, success bool) {
	if proxyURL == "" {
		return
	}
	if success {
		// Common case: nothing is tracked for this proxy — check under a
		// shared lock before taking the exclusive one.
		proxyPoolHealthLock.RLock()
		_, tracked := proxyPoolHealth[proxyURL]
		proxyPoolHealthLock.RUnlock()
		if !tracked {
			return
		}
		proxyPoolHealthLock.Lock()
		delete(proxyPoolHealth, proxyURL)
		proxyPoolHealthLock.Unlock()
		return
	}
	proxyPoolHealthLock.Lock()
	defer proxyPoolHealthLock.Unlock()
	state := proxyPoolHealth[proxyURL]
	state.streak++
	state.until = common.GetTimestamp() + ProxyCooldownSeconds(state.streak)
	proxyPoolHealth[proxyURL] = state
}

// PruneProxyPoolHealth drops health entries for proxies that are no longer
// part of the pool, so the map cannot grow without bound.
func PruneProxyPoolHealth() {
	current := make(map[string]bool)
	for _, entry := range ProxyPoolAllEntries() {
		current[entry.URL] = true
	}
	proxyPoolHealthLock.Lock()
	defer proxyPoolHealthLock.Unlock()
	for proxyURL := range proxyPoolHealth {
		if !current[proxyURL] {
			delete(proxyPoolHealth, proxyURL)
		}
	}
}

// ResolveProxyBinding resolves a stored proxy binding to a concrete proxy
// URL. Pool bindings pick a healthy member; any other value (direct, a
// concrete URL, garbage) passes through unchanged.
func ResolveProxyBinding(binding string) string {
	if resolved, ok := PickProxyPoolMember(binding); ok {
		return resolved
	}
	return binding
}

// IsProxyPoolBinding reports whether the binding references the global pool.
func IsProxyPoolBinding(binding string) bool {
	return binding == "pool" || strings.HasPrefix(binding, "pool:")
}

// PickProxyPoolMember resolves a pool binding to one proxy URL from the
// enabled entries. "pool" rotates randomly; "pool:url:<url>" pins one entry
// and transfers to another healthy member while it cools down, returning once
// it recovers. A fully cooled pool falls back to the entry that expires first.
func PickProxyPoolMember(binding string) (string, bool) {
	if !IsProxyPoolBinding(binding) {
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
	randomHealthy := func() (string, bool) {
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

	if binding == "pool" {
		return randomHealthy()
	}

	pinnedIndex := -1
	if pinnedURL, ok := strings.CutPrefix(binding, ProxyPoolBindingPinnedPrefix); ok {
		pinnedURL = strings.TrimSpace(pinnedURL)
		for i, entry := range entries {
			if entry == pinnedURL {
				pinnedIndex = i
				break
			}
		}
		if pinnedIndex < 0 {
			// The pinned entry was removed or disabled: fall back to rotation.
			return randomHealthy()
		}
	} else {
		// Legacy numeric binding pool:<n>.
		index, err := strconv.Atoi(strings.TrimPrefix(binding, "pool:"))
		if err != nil || index < 0 || index >= len(entries) {
			return randomHealthy()
		}
		pinnedIndex = index
	}

	if healthy(pinnedIndex) {
		return entries[pinnedIndex], true
	}
	for i := 1; i < len(entries); i++ {
		candidate := (pinnedIndex + i) % len(entries)
		if healthy(candidate) {
			return entries[candidate], true
		}
	}
	return entries[earliest()], true
}
