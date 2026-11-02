package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
)

// setProxyPoolForTest swaps the global pool document for one test.
func setProxyPoolForTest(t *testing.T, document string) {
	t.Helper()
	previous := proxyPoolSetting.Proxies
	proxyPoolSetting.Proxies = document
	t.Cleanup(func() { proxyPoolSetting.Proxies = previous })
	proxyPoolHealthLock.Lock()
	proxyPoolHealth = make(map[string]proxyHealthState)
	proxyPoolHealthLock.Unlock()
}

func markForTest(proxy string, success bool) {
	proxyPoolHealthLock.Lock()
	defer proxyPoolHealthLock.Unlock()
	if success {
		delete(proxyPoolHealth, proxy)
		return
	}
	state := proxyPoolHealth[proxy]
	state.streak++
	state.until = common.GetTimestamp() + ProxyCooldownSeconds(state.streak)
	proxyPoolHealth[proxy] = state
}

func TestProxyPoolDocumentParsing(t *testing.T) {
	// The legacy line-based document is upgraded to entries on read.
	entries := ParseProxyPoolDocument("# comment\n socks5://a:1080 \n\nhttp://b:8080\n")
	require.Len(t, entries, 2)
	assert.Equal(t, "socks5://a:1080", entries[0].URL)
	assert.True(t, entries[0].Enabled)
	assert.Equal(t, "http://b:8080", entries[1].URL)

	// The JSON array form carries notes and the enabled flag.
	document := `[{"url":"socks5://user:pass@a:1080","note":"美国机房","enabled":true},{"url":"http://b:8080","enabled":false}]`
	parsed := ParseProxyPoolDocument(document)
	require.Len(t, parsed, 2)
	assert.Equal(t, "美国机房", parsed[0].Note)
	assert.False(t, parsed[1].Enabled)

	setProxyPoolForTest(t, document)
	assert.Equal(t, []string{"socks5://user:pass@a:1080"}, ProxyPoolEntries())
	assert.True(t, ProxyPoolHasURL("http://b:8080"))
	_, enabled := FindProxyPoolEntry("http://b:8080")
	require.True(t, enabled)

	valid, err := ValidateProxyPoolDocument(`[{"url":"direct","enabled":true},{"url":"socks5://user:pass@a:1080","enabled":true}]`)
	require.NoError(t, err)
	require.Len(t, valid, 2)
	assert.Equal(t, "direct", valid[0].URL)

	_, err = ValidateProxyPoolDocument(`[{"url":"ftp://nope:21","enabled":true}]`)
	assert.Error(t, err)
}

func TestProxyPoolSerializeRoundTrip(t *testing.T) {
	entries := []ProxyPoolEntry{
		{URL: "socks5://10.3.0.1:1080", Note: "primary", Enabled: true},
		{URL: "http://10.3.0.2:8080", Enabled: false},
	}
	document := SerializeProxyPoolDocument(entries)
	parsed := ParseProxyPoolDocument(document)
	assert.Equal(t, entries, parsed)
	assert.Equal(t, "", SerializeProxyPoolDocument(nil))
}

func TestProxyCooldownBackoff(t *testing.T) {
	assert.EqualValues(t, 15, ProxyCooldownSeconds(1))
	assert.EqualValues(t, 30, ProxyCooldownSeconds(2))
	assert.EqualValues(t, 60, ProxyCooldownSeconds(3))
	assert.EqualValues(t, ProxyCooldownMaxSeconds, ProxyCooldownSeconds(5))
	assert.EqualValues(t, ProxyCooldownMaxSeconds, ProxyCooldownSeconds(40))
}

func TestPickProxyPoolMemberRandomAvoidsCoolingEntries(t *testing.T) {
	setProxyPoolForTest(t, "socks5://10.0.0.1:1080\nsocks5://10.0.0.2:1080\nsocks5://10.0.0.3:1080")

	for range 10 {
		member, ok := PickProxyPoolMember("pool")
		require.True(t, ok)
		assert.Contains(t, ProxyPoolEntries(), member)
	}

	// Two members cooling leaves only the third available.
	markForTest("socks5://10.0.0.1:1080", false)
	markForTest("socks5://10.0.0.2:1080", false)
	for range 8 {
		member, ok := PickProxyPoolMember("pool")
		require.True(t, ok)
		assert.Equal(t, "socks5://10.0.0.3:1080", member)
	}

	// A recovered member rejoins rotation.
	markForTest("socks5://10.0.0.1:1080", true)
	seen := map[string]bool{}
	for range 60 {
		member, ok := PickProxyPoolMember("pool")
		require.True(t, ok)
		seen[member] = true
	}
	assert.True(t, seen["socks5://10.0.0.1:1080"])
}

func TestPickProxyPoolMemberPinnedByURL(t *testing.T) {
	setProxyPoolForTest(t, `[{"url":"http://10.1.0.1:8080","enabled":true},{"url":"http://10.1.0.2:8080","enabled":true},{"url":"http://10.1.0.3:8080","enabled":false}]`)
	pinned := ProxyPoolBindingPinnedPrefix + "http://10.1.0.2:8080"

	member, ok := PickProxyPoolMember(pinned)
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.2:8080", member)

	// While the pinned entry cools down traffic moves to another enabled one.
	markForTest("http://10.1.0.2:8080", false)
	member, ok = PickProxyPoolMember(pinned)
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.1:8080", member)

	// Recovery returns the pinned entry.
	markForTest("http://10.1.0.2:8080", true)
	member, ok = PickProxyPoolMember(pinned)
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.2:8080", member)

	// A disabled entry is never selected, and pinning it falls back to rotation.
	member, ok = PickProxyPoolMember(ProxyPoolBindingPinnedPrefix + "http://10.1.0.3:8080")
	require.True(t, ok)
	assert.NotEqual(t, "http://10.1.0.3:8080", member)

	// A removed entry also falls back instead of failing.
	member, ok = PickProxyPoolMember(ProxyPoolBindingPinnedPrefix + "http://10.9.9.9:8080")
	require.True(t, ok)
	assert.Contains(t, []string{"http://10.1.0.1:8080", "http://10.1.0.2:8080"}, member)
}

func TestPickProxyPoolMemberLegacyNumericPin(t *testing.T) {
	setProxyPoolForTest(t, "http://10.1.0.1:8080\nhttp://10.1.0.2:8080")

	member, ok := PickProxyPoolMember("pool:1")
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.2:8080", member)
}

func TestPickProxyPoolMemberRejectsOtherBindings(t *testing.T) {
	setProxyPoolForTest(t, "socks5://10.2.0.1:1080")

	_, ok := PickProxyPoolMember("")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("direct")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("socks5://10.2.0.9:1080")
	assert.False(t, ok)

	// Malformed or out-of-range pins fall back to rotation rather than failing.
	member, ok := PickProxyPoolMember("pool:notanumber")
	require.True(t, ok)
	assert.Equal(t, "socks5://10.2.0.1:1080", member)
	member, ok = PickProxyPoolMember("pool:5")
	require.True(t, ok)
	assert.Equal(t, "socks5://10.2.0.1:1080", member)

	// An empty pool cannot resolve anything.
	setProxyPoolForTest(t, "")
	_, ok = PickProxyPoolMember("pool")
	assert.False(t, ok)
}
