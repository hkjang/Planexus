package server

import "testing"

func TestMCPToolContractsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range mcpTools() {
		if tool.Name == "" || seen[tool.Name] {
			t.Fatalf("invalid tool %q", tool.Name)
		}
		seen[tool.Name] = true
		if tool.InputSchema["type"] != "object" {
			t.Fatalf("tool %s has invalid input schema", tool.Name)
		}
	}
}
