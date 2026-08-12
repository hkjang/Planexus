package server

import (
	"testing"

	"github.com/google/uuid"
)

func TestNotificationConditions(t *testing.T) {
	candidate := notificationCandidate{ID: uuid.New(), Value: 82}
	if !matchesNotification(notificationRule{Operator: "lt", Threshold: "90"}, candidate) {
		t.Fatal("expected KPI threshold match")
	}
	if matchesNotification(notificationRule{Operator: "gte", Threshold: "90"}, candidate) {
		t.Fatal("unexpected KPI threshold match")
	}
	risk := notificationCandidate{Value: 3, TextValue: "high"}
	if !matchesNotification(notificationRule{ConditionField: "risk", Operator: "gte", Threshold: "high"}, risk) {
		t.Fatal("expected risk threshold match")
	}
}
