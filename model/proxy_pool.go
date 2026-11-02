package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// ValidateProxyBinding validates a proxy binding used by a channel or a key.
// Bindings address the single global proxy pool:
//
//	""        inherit (key -> channel proxy; channel -> no proxy)
//	"direct"  dial without any proxy
//	"pool"    random rotation over the enabled pool entries
//	"pool:url:<url>"  pin one pool entry by URL
//	"pool:<n>"        legacy numeric pin (still accepted)
//	<url>     a concrete proxy URL
func ValidateProxyBinding(binding string) error {
	b := strings.TrimSpace(binding)
	if b == "" || b == MultiKeyProxyDirect {
		return nil
	}
	if b == "pool" {
		if len(operation_setting.ProxyPoolEntries()) == 0 {
			return fmt.Errorf("the global proxy pool has no enabled entries")
		}
		return nil
	}
	if pinnedURL, isPinned := strings.CutPrefix(b, operation_setting.ProxyPoolBindingPinnedPrefix); isPinned {
		entry, ok := operation_setting.FindProxyPoolEntry(pinnedURL)
		if !ok {
			return fmt.Errorf("proxy %q is not in the global proxy pool", pinnedURL)
		}
		if !entry.Enabled {
			return fmt.Errorf("proxy %q is disabled in the global proxy pool", pinnedURL)
		}
		return nil
	}
	if rest, isNumeric := strings.CutPrefix(b, "pool:"); isNumeric {
		index, err := strconv.Atoi(rest)
		if err != nil || index < 0 {
			return fmt.Errorf("invalid pool member index in binding %q", b)
		}
		if entries := operation_setting.ProxyPoolEntries(); index >= len(entries) {
			return fmt.Errorf("pool member %d is out of range (pool has %d enabled entries)", index, len(entries))
		}
		return nil
	}
	_, err := common.ParseProxyURLStrict(b)
	return err
}

// ValidateKeyProxyBinding restricts key-level bindings to the global pool or
// direct dialing: keys may not carry a manually typed proxy URL.
func ValidateKeyProxyBinding(binding string) error {
	b := strings.TrimSpace(binding)
	if b == "" || b == MultiKeyProxyDirect {
		return nil
	}
	if !operation_setting.IsProxyPoolBinding(b) {
		return fmt.Errorf("key proxies must use the global proxy pool or direct")
	}
	return ValidateProxyBinding(b)
}
