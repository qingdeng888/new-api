package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type proxyPoolRequest struct {
	Name    string `json:"name"`
	Proxies string `json:"proxies"`
}

// GetProxyPools lists every proxy pool for the admin console.
func GetProxyPools(c *gin.Context) {
	pools, err := model.GetAllProxyPools()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, pools)
}

// GetProxyPool returns one proxy pool by id.
func GetProxyPool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pool, err := model.GetProxyPoolById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, pool)
}

// CreateProxyPool validates and stores a new proxy pool.
func CreateProxyPool(c *gin.Context) {
	request := &proxyPoolRequest{}
	if err := c.ShouldBindJSON(request); err != nil {
		common.ApiError(c, err)
		return
	}
	pool := &model.ProxyPool{
		Name:        request.Name,
		Proxies:     request.Proxies,
		CreatedTime: common.GetTimestamp(),
	}
	if err := model.CreateProxyPool(pool); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshProxyPoolCache()
	recordManageAudit(c, "proxy_pool.create", map[string]any{"pool_id": pool.Id, "name": pool.Name})
	common.ApiSuccess(c, pool)
}

// UpdateProxyPool replaces the name and proxy list of an existing pool.
func UpdateProxyPool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pool, err := model.GetProxyPoolById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	request := &proxyPoolRequest{}
	if err := c.ShouldBindJSON(request); err != nil {
		common.ApiError(c, err)
		return
	}
	pool.Name = request.Name
	pool.Proxies = request.Proxies
	if err := model.UpdateProxyPool(pool); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshProxyPoolCache()
	recordManageAudit(c, "proxy_pool.update", map[string]any{"pool_id": pool.Id, "name": pool.Name})
	common.ApiSuccess(c, pool)
}

// DeleteProxyPool removes a proxy pool; bindings referencing it stay in
// place but resolve to nothing until they are re-pointed.
func DeleteProxyPool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteProxyPool(id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshProxyPoolCache()
	recordManageAudit(c, "proxy_pool.delete", map[string]any{"pool_id": id})
	common.ApiSuccess(c, nil)
}
