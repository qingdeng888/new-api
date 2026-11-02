package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// GetChannelProxyPool returns the global proxy pool entries so channel editors
// and the key management dialog can offer a proxy picker. It exposes entries
// only (no save capability) and requires channel read permission.
func GetChannelProxyPool(c *gin.Context) {
	entries := operation_setting.ProxyPoolAllEntries()
	common.ApiSuccess(c, entries)
}
