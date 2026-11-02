package service

import (
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Proxy pool bindings ("pool:<id>:random|fixed:<n>") resolve to one
// concrete proxy URL per request. Transport failures put a proxy endpoint
// into an exponentially growing cool-down (15s doubling to 120s, mirroring
// opencode2api's gateway); resolution skips cooled proxies so traffic
// automatically transfers to the remaining members, and a successful request
// through a proxy clears its state. The cool-down is keyed by proxy URL and
// shared by every pool that contains it.

const (
	proxyCooldownBaseSeconds int64 = 15
	proxyCooldownMaxSeconds  int64 = 120
)

type proxyHealthState struct {
	until  int64
	streak int
}

var (
	proxyHealthLock sync.Mutex
	proxyHealth     = make(map[string]proxyHealthState)
)

func proxyCooldownSeconds(streak int) int64 {
	if streak < 1 {
		streak = 1
	}
	seconds := proxyCooldownBaseSeconds
	for range streak - 1 {
		seconds *= 2
		if seconds >= proxyCooldownMaxSeconds {
			return proxyCooldownMaxSeconds
		}
	}
	return seconds
}

func isProxyCooling(proxyURL string, now int64) bool {
	state, ok := proxyHealth[proxyURL]
	return ok && state.until > now
}

// earliestCoolingProxy returns the member whose cool-down expires first, so
// a fully cooled pool still hands out a proxy instead of failing outright.
func earliestCoolingProxy(members []string, now int64) int {
	earliest := 0
	earliestUntil := int64(-1)
	for i, member := range members {
		until := int64(0)
		if state, ok := proxyHealth[member]; ok {
			until = state.until
		}
		if earliestUntil < 0 || until < earliestUntil {
			earliestUntil = until
			earliest = i
		}
	}
	_ = now
	return earliest
}

// ResolvePoolProxyBinding resolves a "pool:" binding to one concrete proxy
// URL. ok is false when the binding is not a pool reference or the pool is
// missing/empty (the caller then keeps the raw binding).
func ResolvePoolProxyBinding(binding string) (string, bool) {
	rest, ok := strings.CutPrefix(binding, "pool:")
	if !ok {
		return "", false
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return "", false
	}
	poolId, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", false
	}
	pool, ok := model.GetCachedProxyPool(poolId)
	if !ok {
		return "", false
	}
	members := pool.ProxyList()
	if len(members) == 0 {
		return "", false
	}

	proxyHealthLock.Lock()
	defer proxyHealthLock.Unlock()
	now := time.Now().Unix()
	healthy := func(idx int) bool {
		return !isProxyCooling(members[idx], now)
	}

	switch parts[1] {
	case "random":
		var healthyIdx []int
		for i := range members {
			if healthy(i) {
				healthyIdx = append(healthyIdx, i)
			}
		}
		if len(healthyIdx) > 0 {
			return members[healthyIdx[rand.Intn(len(healthyIdx))]], true
		}
		return members[earliestCoolingProxy(members, now)], true

	case "fixed":
		if len(parts) != 3 {
			return "", false
		}
		fixedIdx, err := strconv.Atoi(parts[2])
		if err != nil || fixedIdx < 0 || fixedIdx >= len(members) {
			return "", false
		}
		// The pinned member is preferred; when it cools down traffic
		// transfers to the next healthy member and returns once it recovers.
		if healthy(fixedIdx) {
			return members[fixedIdx], true
		}
		for i := 1; i < len(members); i++ {
			idx := (fixedIdx + i) % len(members)
			if healthy(idx) {
				return members[idx], true
			}
		}
		return members[earliestCoolingProxy(members, now)], true
	}
	return "", false
}

// MarkProxyTransportResult records the outcome of an outbound request made
// through proxyURL so pool resolution can route around failing proxies.
func MarkProxyTransportResult(proxyURL string, success bool) {
	if proxyURL == "" {
		return
	}
	proxyHealthLock.Lock()
	defer proxyHealthLock.Unlock()
	if success {
		delete(proxyHealth, proxyURL)
		return
	}
	state := proxyHealth[proxyURL]
	state.streak++
	state.until = time.Now().Unix() + proxyCooldownSeconds(state.streak)
	proxyHealth[proxyURL] = state
	if state.streak == 1 {
		common.SysLog(common.LogText("proxy entering cool-down: %s (%ds)", common.RedactProxyURL(proxyURL), state.until-time.Now().Unix()))
	}
}
