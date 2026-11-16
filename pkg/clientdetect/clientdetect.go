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

// Package clientdetect names the coding agents that call the relay, so a token
// can be narrowed to the clients its owner means to serve.
//
// Every signal here is a request header, which the caller controls: this is a
// guard against a shared or misused key, not an authentication factor. A
// gateway that rewrites headers hides the client entirely, which the caller
// policy treats as an unknown client.
package clientdetect

import (
	"net/http"
	"slices"
	"strings"
)

// Kind is one recognized client. The value is the token's stored identifier, so
// changing one invalidates saved token restrictions.
type Kind string

const (
	// KindCodex is the OpenAI Codex CLI.
	KindCodex Kind = "codex"
	// KindClaudeCode is Anthropic's Claude Code CLI.
	KindClaudeCode Kind = "claude_code"
	// KindDeepSeekHarness is the DeepSeek Harness CLI, which reaches the relay
	// through pi-ai.
	KindDeepSeekHarness Kind = "deepseek_harness"
	// KindHermes is Nous Research's Hermes Agent.
	KindHermes Kind = "hermes"
)

// allClients is the console order of every supported client.
var allClients = []Kind{KindCodex, KindClaudeCode, KindDeepSeekHarness, KindHermes}

// All returns every supported client in console order.
func All() []Kind {
	return slices.Clone(allClients)
}

// IsKnown reports whether value names a supported client.
func IsKnown(value string) bool {
	return slices.Contains(allClients, Kind(value))
}

// ParseList keeps the supported clients named by a comma separated token value,
// in console order and without duplicates. Unknown names are dropped so a stale
// stored value cannot lock a token out of every client.
func ParseList(value string) []Kind {
	if value == "" {
		return nil
	}
	named := make(map[Kind]bool)
	for part := range strings.SplitSeq(value, ",") {
		named[Kind(strings.TrimSpace(part))] = true
	}
	parsed := make([]Kind, 0, len(named))
	for _, kind := range allClients {
		if named[kind] {
			parsed = append(parsed, kind)
		}
	}
	return parsed
}

// FormatList renders clients as the comma separated token value.
func FormatList(kinds []Kind) string {
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, string(kind))
	}
	return strings.Join(parts, ",")
}

// Detect names the client behind a request. It reports false for a request whose
// headers match no supported client, including one whose headers were rewritten
// on the way in.
func Detect(h http.Header) (Kind, bool) {
	if h == nil {
		return "", false
	}
	userAgent := strings.ToLower(strings.TrimSpace(h.Get("User-Agent")))

	// DeepSeek Harness sends "deepseek-harness/<version> (+<repository>)" on
	// every provider request, model discovery included. Its adapters set that
	// header last, over whatever the underlying client would send, so the
	// product name is always the harness itself.
	if strings.HasPrefix(userAgent, "deepseek-harness/") || userAgent == "deepseek-harness" {
		return KindDeepSeekHarness, true
	}

	if isClaudeCLI(userAgent) {
		return KindClaudeCode, true
	}

	// Codex states its product in the originator header, and repeats it as the
	// user agent product token.
	originator := strings.ToLower(strings.TrimSpace(h.Get("originator")))
	if strings.HasPrefix(originator, "codex") || strings.HasPrefix(userAgent, "codex_cli_rs/") ||
		strings.HasPrefix(userAgent, "codex-tui/") {
		return KindCodex, true
	}

	if userAgent == "hermes-cli" || strings.HasPrefix(userAgent, "hermes-cli/") ||
		strings.HasPrefix(userAgent, "hermes-agent/") {
		return KindHermes, true
	}
	return "", false
}

// Allowed reports whether a token restricted to allowed admits a request, and
// names the client it came from. An unrecognized request is refused: a
// restriction that admitted whatever it could not identify would not be one.
func Allowed(allowed []Kind, h http.Header) (Kind, bool) {
	client, recognized := Detect(h)
	if !recognized || !slices.Contains(allowed, client) {
		return client, false
	}
	return client, true
}

// isClaudeCLI matches the "claude-cli/<version> (external, cli)" user agent.
func isClaudeCLI(userAgent string) bool {
	return userAgent == "claude-cli" || strings.HasPrefix(userAgent, "claude-cli/")
}
