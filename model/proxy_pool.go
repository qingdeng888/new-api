package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ProxyPool is a named collection of outbound proxy URLs that channel-level
// or per-key proxy bindings can reference (pool:<id>:<mode>).
type ProxyPool struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"type:varchar(64);uniqueIndex;not null"`
	Proxies     string `json:"proxies" gorm:"type:text"` // one proxy URL per line; blank lines and #-comments ignored
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
}

func (ProxyPool) TableName() string {
	return "proxy_pools"
}

func (pool *ProxyPool) ProxyList() []string {
	var proxies []string
	for _, raw := range strings.Split(pool.Proxies, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxies = append(proxies, line)
	}
	return proxies
}

func GetAllProxyPools() ([]*ProxyPool, error) {
	var pools []*ProxyPool
	err := DB.Order("id asc").Find(&pools).Error
	return pools, err
}

func GetProxyPoolById(id int) (*ProxyPool, error) {
	var pool ProxyPool
	err := DB.First(&pool, id).Error
	if err != nil {
		return nil, err
	}
	return &pool, nil
}

func IsProxyPoolNameTaken(name string, excludeId int) bool {
	var count int64
	query := DB.Model(&ProxyPool{}).Where("name = ?", name)
	if excludeId > 0 {
		query = query.Where("id != ?", excludeId)
	}
	res := query.Count(&count)
	if res.Error != nil {
		// Fail-closed: treat DB errors as name taken to prevent conflicts.
		return true
	}
	return count > 0
}

func CreateProxyPool(pool *ProxyPool) error {
	if err := validateProxyPool(pool); err != nil {
		return err
	}
	return DB.Create(pool).Error
}

func UpdateProxyPool(pool *ProxyPool) error {
	if err := validateProxyPool(pool); err != nil {
		return err
	}
	return DB.Save(pool).Error
}

func DeleteProxyPool(id int) error {
	return DB.Delete(&ProxyPool{}, id).Error
}

func validateProxyPool(pool *ProxyPool) error {
	pool.Name = strings.TrimSpace(pool.Name)
	if pool.Name == "" {
		return errors.New("proxy pool name is required")
	}
	if len(pool.Name) > 64 {
		return errors.New("proxy pool name is too long")
	}
	proxies := pool.ProxyList()
	if len(proxies) == 0 {
		return errors.New("proxy pool needs at least one proxy")
	}
	for _, proxy := range proxies {
		if _, err := common.ParseProxyURLStrict(proxy); err != nil {
			return fmt.Errorf("invalid proxy %q: %w", proxy, err)
		}
	}
	pool.Proxies = strings.Join(proxies, "\n")
	return nil
}

// Cached pool definitions: pools change rarely and every relay request with
// a pool binding reads them, so they live in a lock-guarded in-memory map
// rebuilt on CRUD (following the channel cache pattern).
var proxyPoolLock sync.RWMutex
var proxyPoolsById = make(map[int]*ProxyPool)

// InitProxyPoolCache loads every proxy pool into memory.
func InitProxyPoolCache() error {
	pools, err := GetAllProxyPools()
	if err != nil {
		return err
	}
	proxyPoolLock.Lock()
	defer proxyPoolLock.Unlock()
	proxyPoolsById = make(map[int]*ProxyPool, len(pools))
	for _, pool := range pools {
		proxyPoolsById[pool.Id] = pool
	}
	return nil
}

// RefreshProxyPoolCache rebuilds the in-memory pool cache after a CRUD write.
func RefreshProxyPoolCache() {
	if err := InitProxyPoolCache(); err != nil {
		common.SysLog(common.LogText("failed to refresh proxy pool cache: %v", err))
	}
}

// GetCachedProxyPool returns a cached pool definition by id.
func GetCachedProxyPool(id int) (*ProxyPool, bool) {
	proxyPoolLock.RLock()
	defer proxyPoolLock.RUnlock()
	pool, ok := proxyPoolsById[id]
	return pool, ok
}

// ValidateProxyBinding validates a proxy binding used by a channel or a
// multi-key entry: empty inherits, "direct" dials without a proxy, "pool:"
// references a proxy pool, anything else must be a concrete proxy URL.
func ValidateProxyBinding(binding string) error {
	b := strings.TrimSpace(binding)
	if b == "" || b == MultiKeyProxyDirect {
		return nil
	}
	rest, isPool := strings.CutPrefix(b, "pool:")
	if !isPool {
		_, err := common.ParseProxyURLStrict(b)
		return err
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid pool binding %q", b)
	}
	poolId, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid pool id in binding %q", b)
	}
	switch parts[1] {
	case "random", "ordered":
		if len(parts) != 2 {
			return fmt.Errorf("invalid pool binding %q", b)
		}
	case "fixed":
		if len(parts) != 3 {
			return fmt.Errorf("invalid pool binding %q", b)
		}
		if index, err := strconv.Atoi(parts[2]); err != nil || index < 0 {
			return fmt.Errorf("invalid pool member index in binding %q", b)
		}
	default:
		return fmt.Errorf("unknown pool mode in binding %q", b)
	}
	if _, ok := GetCachedProxyPool(poolId); !ok {
		return fmt.Errorf("proxy pool %d does not exist", poolId)
	}
	return nil
}

// SyncProxyPoolCache periodically rebuilds the pool cache so edits made on
// another node become visible.
func SyncProxyPoolCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog(common.LogText("syncing proxy pool cache"))
		RefreshProxyPoolCache()
	}
}
