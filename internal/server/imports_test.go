package server

import "testing"

func TestImportMappingAndValidation(t *testing.T) {
	mapping, problems := resolveImportMapping(importFields["kpi"], []string{"KPI Code", "KPI명", "목표", "실적"}, nil)
	if len(problems) != 0 {
		t.Fatalf("unexpected mapping errors: %#v", problems)
	}
	if mapping["code"] != 0 || mapping["name"] != 1 || mapping["target"] != 2 {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}
	problems = validateImportRow("kpi", 2, map[string]string{"code": "REV-1", "name": "Revenue", "target": "not-number"})
	if len(problems) != 1 || problems[0].Field != "target" {
		t.Fatalf("expected target validation: %#v", problems)
	}
}

func TestSafeImportFileName(t *testing.T) {
	if value := safeFileName(`..\folder\kpi.xlsx`); value != "kpi.xlsx" {
		t.Fatalf("unexpected safe name: %q", value)
	}
}
