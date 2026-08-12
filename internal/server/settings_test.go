package server

import "testing"

func TestValidateKnownSettings(t *testing.T) {
	tests := []struct {
		name     string
		category string
		key      string
		value    string
		valid    bool
	}{
		{"session valid", "system", "session", `{"timeoutMinutes":120}`, true},
		{"session too long", "system", "session", `{"timeoutMinutes":721}`, false},
		{"API rate valid", "system", "api_rate_limit", `{"requestsPerMinute":600}`, true},
		{"API rate invalid", "system", "api_rate_limit", `{"requestsPerMinute":10}`, false},
		{"key policy valid", "security", "personal_keys", `{"maxLifetimeDays":30,"rotationOverlapHours":12,"allowedScopes":["strategy:read"]}`, true},
		{"key policy bad scope", "security", "personal_keys", `{"maxLifetimeDays":30,"rotationOverlapHours":12,"allowedScopes":["INVALID"]}`, false},
		{"MCP policy valid", "integration", "mcp", `{"enabled":true,"allowedTools":["get_strategy"]}`, true},
		{"MCP unknown tool", "integration", "mcp", `{"enabled":true,"allowedTools":["delete_everything"]}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateKnownSetting(test.category, test.key, []byte(test.value))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
