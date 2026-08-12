package server

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

func TestPlanAccessDoesNotExpandOwnPermissionToOrganization(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	organizationID := uuid.New()
	p := auth.Principal{ID: userID, OrganizationID: &organizationID, Permissions: []string{"plan:own"}}
	if !canReadPlan(p, &userID, &organizationID, "internal") {
		t.Fatal("owner should read their own plan")
	}
	if canReadPlan(p, &otherUserID, &organizationID, "internal") {
		t.Fatal("plan:own must not expose another user's plan in the same organization")
	}
}

func TestPlanAccessAppliesClassificationABAC(t *testing.T) {
	userID := uuid.New()
	organizationID := uuid.New()
	p := auth.Principal{ID: userID, OrganizationID: &organizationID, Permissions: []string{"plan:own"}}
	if canReadPlan(p, &userID, &organizationID, "executive") {
		t.Fatal("non-executive owner must not bypass executive classification")
	}
	p.Roles = []string{"executive"}
	if !canReadPlan(p, &userID, &organizationID, "executive") {
		t.Fatal("executive owner should read their executive-classified plan")
	}
}
