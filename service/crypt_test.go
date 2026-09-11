package service

import (
	"strings"
	"sync"
	"testing"
)

// Reference vectors generated with PostgreSQL pgcrypto (contrib/pgcrypto
// crypt-des.c) in the bayeos dev database:
//
//	CREATE EXTENSION pgcrypto;
//	SELECT crypt('password', 'Sa');
//
// Only the traditional DES scheme is covered; the truncated vector proves the
// 8-character key limit is honoured.
func TestCryptDES(t *testing.T) {
	var tests = []struct {
		password string
		salt     string
		want     string
	}{
		{"password", "Sa", "SaIQ1RiNlnVxc"},
		{"oer", "ol", "oltpSI7itnfD6"},
		{"oliver", "ol", "olPAJKnUgWj/E"},
		{"12345678", "12", "12yJ.Of/NQ.Pk"},
		{"123456789", "12", "12yJ.Of/NQ.Pk"},
		{"secret", "se", "sefjKaLm7zybE"},
		{"", "..", "..X8NBuQ4l6uQ"},
		{"a", "..", "..4/MD05HLWjI"},
		{"test", "80", "80uqPkqkEvWEk"},
		{"TopSecret", "To", "ToqU3XAI7SULY"},
		{"correct horse battery staple", "co", "coo1MA4ZbcqIU"},
		{"abc", "xy", "xy7MmN0ZLly8E"},
		{"Qwerty!", "Qw", "QwRnRUqlzAlYw"},
	}

	for _, tt := range tests {
		got, err := CryptDES(tt.password, tt.salt)
		if err != nil {
			t.Errorf("CryptDES(%q, %q) returned error: %v", tt.password, tt.salt, err)
			continue
		}
		if got != tt.want {
			t.Errorf("CryptDES(%q, %q) = %q, want %q", tt.password, tt.salt, got, tt.want)
		}
		if len(got) != 13 || !strings.HasPrefix(got, tt.salt) {
			t.Errorf("CryptDES(%q, %q) = %q: want 13 chars prefixed with salt", tt.password, tt.salt, got)
		}
	}
}

func TestCryptDESInvalidSalt(t *testing.T) {
	for _, salt := range []string{"", "x"} {
		if _, err := CryptDES("password", salt); err == nil {
			t.Errorf("CryptDES with salt %q should return an error", salt)
		}
	}
}

func TestEncodeDES(t *testing.T) {
	s := &userService{}

	tests := []struct {
		password string
		salt     string
		want     string
	}{
		{"oliver", "ol", "olPAJKnUgWj/E"},
		{"oer", "ol", "oltpSI7itnfD6"},
		{"password", "pa", "papAq5PwY/QQM"},
		{"123456789", "12", "12yJ.Of/NQ.Pk"}, // 9 chars: key truncated to 8
	}
	for _, tt := range tests {
		got, err := s.encodeDES(tt.password, tt.salt)
		if err != nil {
			t.Errorf("encodeDES(%q, %q) returned error: %v", tt.password, tt.salt, err)
			continue
		}
		if got != tt.want {
			t.Errorf("encodeDES(%q, %q) = %q, want %q", tt.password, tt.salt, got, tt.want)
		}
	}

	// Verification semantics as in SignIn: salt is taken from the stored hash
	// prefix, so recomputing with stored[:2] must reproduce the stored hash.
	stored := "olPAJKnUgWj/E"
	if got, err := s.encodeDES("oliver", stored[:2]); err != nil || got != stored {
		t.Errorf("encodeDES(\"oliver\", stored[:2]) = %q, %v; want %q", got, err, stored)
	}

	for _, short := range []string{"", "x"} {
		if _, err := s.encodeDES(short, "ol"); err == nil {
			t.Errorf("encodeDES(%q, \"ol\") should return an error", short)
		}
	}
}

func TestCryptDESConcurrent(t *testing.T) {
	const goroutines = 16
	const iterations = 500

	hashes := []struct {
		password string
		salt     string
		want     string
	}{
		{"oer", "ol", "oltpSI7itnfD6"},
		{"oliver", "ol", "olPAJKnUgWj/E"},
		{"password", "Sa", "SaIQ1RiNlnVxc"},
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, h := range hashes {
					got, err := CryptDES(h.password, h.salt)
					if err != nil || got != h.want {
						t.Errorf("CryptDES(%q, %q) = %q, %v; want %q", h.password, h.salt, got, err, h.want)
					}
				}
			}
		}()
	}
	wg.Wait()
}