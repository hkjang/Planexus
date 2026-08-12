package server

import (
	"testing"
	"time"
)

func TestLoginRateLimitAndReset(t *testing.T) {
	s := &Server{loginAttempts: map[string]loginAttempt{}}
	for range 5 {
		s.loginFailed("127.0.0.1/admin")
	}
	if s.loginBlocked("127.0.0.1/admin") <= 14*time.Minute {
		t.Fatal("expected login block")
	}
	s.loginSucceeded("127.0.0.1/admin")
	if s.loginBlocked("127.0.0.1/admin") != 0 {
		t.Fatal("successful login should reset limiter")
	}
}

func TestAuthenticatedAPIRateLimitWindow(t *testing.T) {
	s := &Server{apiRates: map[string]apiRateBucket{}}
	now := time.Now()
	for range 2 {
		if allowed, _ := s.consumeAPIRate("user-1", 2, now); !allowed {
			t.Fatal("request inside limit was rejected")
		}
	}
	if allowed, retry := s.consumeAPIRate("user-1", 2, now); allowed || retry <= 0 {
		t.Fatal("request over limit was not rejected with retry duration")
	}
	if allowed, _ := s.consumeAPIRate("user-1", 2, now.Add(time.Minute)); !allowed {
		t.Fatal("new rate window did not reset")
	}
}
