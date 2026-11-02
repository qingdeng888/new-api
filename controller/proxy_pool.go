package controller

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// GetChannelProxyPool returns the global proxy pool entries so channel editors
// and the key management dialog can offer a proxy picker. It exposes entries
// only (no save capability) and requires channel read permission.
func GetChannelProxyPool(c *gin.Context) {
	entries := operation_setting.ProxyPoolAllEntries()
	common.ApiSuccess(c, entries)
}

// proxyConnectivityTarget is the fixed endpoint used to check whether a proxy
// can reach the public internet. Any HTTP response proves the proxy works; the
// status code itself is informational.
const proxyConnectivityTarget = "https://www.apple.com"

const (
	proxyConnectivityTimeout     = 10 * time.Second
	proxyConnectivityConcurrency = 8
	proxyConnectivityMaxURLs     = 50
)

type proxyConnectivityRequest struct {
	URLs []string `json:"urls"`
}

type proxyConnectivityResult struct {
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// TestProxyPoolEntries checks connectivity for the requested pool entries.
// Only URLs that belong to the pool are accepted, so the endpoint cannot be
// used to probe arbitrary destinations through an attacker-chosen proxy.
func TestProxyPoolEntries(c *gin.Context) {
	var request proxyConnectivityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if len(request.URLs) == 0 {
		common.ApiErrorT(c, "Select at least one proxy to test")
		return
	}
	if len(request.URLs) > proxyConnectivityMaxURLs {
		common.ApiErrorT(c, "Too many proxies in one test run")
		return
	}

	allowed := make(map[string]bool)
	for _, entry := range operation_setting.ProxyPoolAllEntries() {
		allowed[entry.URL] = true
	}
	urls := make([]string, 0, len(request.URLs))
	for _, raw := range request.URLs {
		proxyURL := strings.TrimSpace(raw)
		if !allowed[proxyURL] {
			common.ApiErrorT(c, "Proxy is not in the global proxy pool")
			return
		}
		urls = append(urls, proxyURL)
	}

	results := checkProxyConnectivity(urls)
	common.ApiSuccess(c, results)
}

func checkProxyConnectivity(urls []string) []proxyConnectivityResult {
	results := make([]proxyConnectivityResult, len(urls))
	semaphore := make(chan struct{}, proxyConnectivityConcurrency)
	var wg sync.WaitGroup
	for i, proxyURL := range urls {
		wg.Add(1)
		go func(index int, target string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			results[index] = probeProxy(target)
		}(i, proxyURL)
	}
	wg.Wait()
	return results
}

func probeProxy(proxyURL string) proxyConnectivityResult {
	result := proxyConnectivityResult{URL: proxyURL}
	started := time.Now()

	clientProxy := proxyURL
	if proxyURL == "direct" {
		clientProxy = ""
	}
	client, err := service.GetHttpClientWithProxySettings(clientProxy, dto.ChannelSettings{})
	if err != nil {
		result.Error = common.RedactProxyURL(strings.TrimSpace(err.Error()))
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), proxyConnectivityTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyConnectivityTarget, nil)
	if err != nil {
		result.Error = "invalid test request"
		return result
	}
	req.Header.Set("User-Agent", "new-api-proxy-check/1.0")

	resp, err := client.Do(req)
	result.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = common.RedactProxyURL(strings.TrimSpace(err.Error()))
		return result
	}
	defer resp.Body.Close()
	// Any HTTP response means the proxy reached the target.
	result.OK = true
	result.Status = resp.StatusCode
	return result
}
