package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// ValidateProxyBinding validates a proxy binding used by a channel or a
// multi-key entry. Bindings address the single global proxy pool:
//
//	""        inherit (key -> channel proxy; channel -> no proxy)
//	"direct"  dial without any proxy
//	"pool"    random rotation over the global pool
//	"pool:<n>"  pinned n-th entry of the global pool (transfers while it cools)
//	<url>     a concrete proxy URL
func ValidateProxyBinding(binding string) error {
	b := strings.TrimSpace(binding)
	if b == "" || b == MultiKeyProxyDirect {
		return nil
	}
	if b == "pool" {
		if len(operation_setting.ProxyPoolEntries()) == 0 {
			return fmt.Errorf("the global proxy pool is empty")
		}
		return nil
	}
	if rest, isPinned := strings.CutPrefix(b, "pool:"); isPinned {
		index, err := strconv.Atoi(rest)
		if err != nil || index < 0 {
			return fmt.Errorf("invalid pool member index in binding %q", b)
		}
		if entries := operation_setting.ProxyPoolEntries(); index >= len(entries) {
			return fmt.Errorf("pool member %d is out of range (pool has %d entries)", index, len(entries))
		}
		return nil
	}
	_, err := common.ParseProxyURLStrict(b)
	return err
}
