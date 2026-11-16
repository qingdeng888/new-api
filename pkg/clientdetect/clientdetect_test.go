/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package clientdetect

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeSDKHeaders are the instrumentation headers the official Anthropic SDK
// adds to every request; pi-ai does not send them.
func claudeSDKHeaders() http.Header {
	return http.Header{
		"User-Agent":          {"claude-cli/2.1.251 (external, cli)"},
		"X-App":               {"cli"},
		"X-Stainless-Lang":    {"js"},
		"X-Stainless-Runtime": {"node"},
	}
}

func TestDetectNamesEachSupportedClient(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   Kind
	}{
		{
			// codex-rs states its product in originator and repeats it as the
			// user agent product token.
			name: "codex cli",
			header: http.Header{
				"Originator": {"codex_cli_rs"},
				"User-Agent": {"codex_cli_rs/0.159.2 (Linux Ubuntu 22.04; x86_64) unknown (codex_cli_rs; 0.159.2)"},
			},
			want: KindCodex,
		},
		{
			name: "codex tui",
			header: http.Header{
				"Originator": {"codex-tui"},
				"User-Agent": {"codex-tui/0.159.2 (macOS 15.1; aarch64) Apple_Terminal/455"},
			},
			want: KindCodex,
		},
		{
			name:   "codex without originator",
			header: http.Header{"User-Agent": {"codex_cli_rs/0.159.2 (Linux 6.1; x86_64)"}},
			want:   KindCodex,
		},
		{
			name:   "claude code",
			header: claudeSDKHeaders(),
			want:   KindClaudeCode,
		},
		{
			// Without the browser-access header a bare claude-cli user agent is
			// assumed to be Claude Code: pi-ai always sets that header, so its
			// absence points at the real CLI.
			name:   "claude cli without stainless headers",
			header: http.Header{"User-Agent": {"claude-cli/2.1.251"}},
			want:   KindClaudeCode,
		},
		{
			// pi-ai with an API key sends its own product name.
			name:   "deepseek harness with api key",
			header: http.Header{"User-Agent": {"pi/0.2.2 (linux; node/v24.22.3; x64)"}},
			want:   KindDeepSeekHarness,
		},
		{
			// On OAuth credentials pi-ai borrows the Claude Code name, but asks
			// for browser access and sends no stainless headers.
			name: "deepseek harness with oauth credential",
			header: http.Header{
				"User-Agent": {"claude-cli/2.1.251"},
				"X-App":      {"cli"},
				"Anthropic-Dangerous-Direct-Browser-Access": {"true"},
			},
			want: KindDeepSeekHarness,
		},
		{
			name:   "hermes with versioned user agent",
			header: http.Header{"User-Agent": {"hermes-cli/0.9.1"}},
			want:   KindHermes,
		},
		{
			name:   "hermes without a resolved version",
			header: http.Header{"User-Agent": {"hermes-cli"}},
			want:   KindHermes,
		},
		{
			// The router provider names the product differently.
			name:   "hermes agent product name",
			header: http.Header{"User-Agent": {"Hermes-Agent/0.9.1"}},
			want:   KindHermes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Detect(tc.header)
			require.True(t, ok, "expected a recognized client")
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetectRefusesUnidentifiedRequests(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
	}{
		{name: "no headers", header: http.Header{}},
		{name: "curl", header: http.Header{"User-Agent": {"curl/8.5.0"}}},
		{name: "openai python sdk", header: http.Header{"User-Agent": {"OpenAI/Python 1.40.0"}, "X-Stainless-Lang": {"python"}}},
		{name: "browser", header: http.Header{"User-Agent": {"Mozilla/5.0 (X11; Linux x86_64) Firefox/131.0"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, ok := Detect(tc.header)
			assert.False(t, ok)
			assert.Empty(t, client)
		})
	}
}

func TestAllowedEnforcesWhitelistSemantics(t *testing.T) {
	codexOnly := []Kind{KindCodex}

	client, allowed := Allowed(codexOnly, http.Header{"Originator": {"codex_cli_rs"}})
	assert.True(t, allowed)
	assert.Equal(t, KindCodex, client)

	client, allowed = Allowed(codexOnly, claudeSDKHeaders())
	assert.False(t, allowed, "a recognized client that is not listed must be refused")
	assert.Equal(t, KindClaudeCode, client)

	client, allowed = Allowed(codexOnly, http.Header{"User-Agent": {"curl/8.5.0"}})
	assert.False(t, allowed, "an unidentified request must be refused")
	assert.Empty(t, client)

	_, allowed = Allowed([]Kind{KindCodex, KindClaudeCode}, claudeSDKHeaders())
	assert.True(t, allowed, "any listed client is admitted")
}

func TestParseListKeepsKnownClientsInConsoleOrder(t *testing.T) {
	assert.Equal(t, []Kind{KindCodex, KindClaudeCode}, ParseList(" claude_code , codex ,claude_code,"))
	assert.Empty(t, ParseList(""))
	assert.Empty(t, ParseList(" , "))
	// A value naming something no longer supported must not narrow the token to
	// an unservable set.
	assert.Equal(t, []Kind{KindHermes}, ParseList("retired_client,hermes"))
}

func TestFormatListAndIsKnown(t *testing.T) {
	assert.Equal(t, "codex,hermes", FormatList([]Kind{KindCodex, KindHermes}))
	assert.True(t, IsKnown("deepseek_harness"))
	assert.False(t, IsKnown("codex_cli"))
	assert.False(t, IsKnown(""))
}
