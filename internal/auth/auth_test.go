package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestPermissionWildcards(t *testing.T) {
	p := Principal{Permissions: []string{"strategy:*", "dashboard:read"}}
	if !p.Has("strategy:read") || !p.Has("strategy:update") || p.Has("project:read") {
		t.Fatal("wildcard permission evaluation failed")
	}
}

func TestRestrictedABACRequiresOrganization(t *testing.T) {
	org := uuid.New()
	other := uuid.New()
	p := Principal{OrganizationID: &org, Roles: []string{"executive"}, Permissions: []string{"strategy:read"}}
	if !p.Can("strategy:read", Resource{OrganizationID: &org, Classification: "restricted"}) {
		t.Fatal("same organization should be allowed")
	}
	if p.Can("strategy:read", Resource{OrganizationID: &other, Classification: "restricted"}) {
		t.Fatal("different organization must be denied")
	}
}

func TestEffectiveKeyScopesTrackCurrentRolePermissions(t *testing.T) {
	got := effectiveKeyScopes([]string{"strategy:read", "project:read", "decision:read"}, []string{"strategy:*", "decision:read"})
	if len(got) != 2 || got[0] != "strategy:read" || got[1] != "decision:read" {
		t.Fatalf("unexpected effective key scopes: %#v", got)
	}
}
