package server

import "testing"

func TestEscapeLike(t *testing.T) {
	if got, want := escapeLike("50%_growth\\plan"), "50\\%\\_growth\\\\plan"; got != want {
		t.Fatalf("escapeLike()=%q, want %q", got, want)
	}
}
