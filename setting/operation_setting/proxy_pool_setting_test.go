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
	entries := SplitProxyPoolEntries("# comment\n socks5://a:1080 \n\nhttp://b:8080\n")
	assert.Equal(t, []string{"socks5://a:1080", "http://b:8080"}, entries)

	valid, err := ValidateProxyPoolDocument("direct\nsocks5://user:pass@a:1080")
	require.NoError(t, err)
	assert.Equal(t, []string{"direct", "socks5://user:pass@a:1080"}, valid)

	_, err = ValidateProxyPoolDocument("socks5://a:1080\nftp://nope:21")
	assert.Error(t, err)
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

func TestPickProxyPoolMemberPinnedTransfersAndRecovers(t *testing.T) {
	setProxyPoolForTest(t, "http://10.1.0.1:8080\nhttp://10.1.0.2:8080")

	member, ok := PickProxyPoolMember("pool:0")
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.1:8080", member)

	markForTest("http://10.1.0.1:8080", false)
	member, ok = PickProxyPoolMember("pool:0")
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.2:8080", member)

	markForTest("http://10.1.0.1:8080", true)
	member, ok = PickProxyPoolMember("pool:0")
	require.True(t, ok)
	assert.Equal(t, "http://10.1.0.1:8080", member)
}

func TestPickProxyPoolMemberRejectsOtherBindings(t *testing.T) {
	setProxyPoolForTest(t, "socks5://10.2.0.1:1080")

	_, ok := PickProxyPoolMember("")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("direct")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("socks5://10.2.0.9:1080")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("pool:notanumber")
	assert.False(t, ok)
	_, ok = PickProxyPoolMember("pool:5")
	assert.False(t, ok)

	// An empty pool cannot resolve anything.
	setProxyPoolForTest(t, "")
	_, ok = PickProxyPoolMember("pool")
	assert.False(t, ok)
}
