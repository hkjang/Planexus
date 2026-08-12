package server

import (
	"strconv"
	"strings"
)

func fmtFloat(value float64) string { return strconv.FormatFloat(value, 'f', 1, 64) }
func fmtInt(value int) string       { return strconv.Itoa(value) }
func validSecurityClassification(value string) bool {
	return oneOf(strings.ToLower(value), "public", "internal", "confidential", "executive", "restricted")
}
