package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingAndRerankValidateJSONBodiesWithoutPanicking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct {
		name   string
		path   string
		format types.RelayFormat
		valid  string
	}{
		{"embedding", "/v1/embeddings", types.RelayFormatEmbedding, `{"model":"text-embedding-3-small","input":"hello"}`},
		{"rerank", "/v1/rerank", types.RelayFormatRerank, `{"model":"rerank-model","query":"hello","documents":["hello world"]}`},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, body := range []struct {
				name  string
				value string
				valid bool
			}{
				{"null", "null", false},
				{"empty object", "{}", false},
				{"malformed JSON", "{", false},
				{"valid request", endpoint.valid, true},
			} {
				t.Run(body.name, func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(body.value))
					c.Request.Header.Set("Content-Type", "application/json")
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					var request dto.Request
					var err error
					require.NotPanics(t, func() { request, err = GetAndValidateRequest(c, endpoint.format) })
					if !body.valid {
						require.Error(t, err)
						assert.Nil(t, request)
						return
					}
					require.NoError(t, err)
					if endpoint.format == types.RelayFormatEmbedding {
						assert.Equal(t, "hello", request.(*dto.EmbeddingRequest).Input)
					} else {
						assert.Equal(t, []any{"hello world"}, request.(*dto.RerankRequest).Documents)
					}
				})
			}
		})
	}
}
