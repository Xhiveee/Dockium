package auth

import (
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		login    string
		wantErr  bool
	}{
		{"valid", "Dockium2026!", "admin", false},
		{"too short", "short1", "admin", true},
		{"letters only", "onlyletters", "admin", true},
		{"digits only", "1234567890", "admin", true},
		{"equals login", "adminadmin1", "adminadmin1", true},
		{"too long", strings.Repeat("a1", 40), "admin", true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePassword(test.password, test.login)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidatePassword() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("Dockium2026!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(hash, "Dockium2026!") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong-password1") {
		t.Fatal("expected wrong password to fail")
	}
	VerifyDummyPassword("anything")
}

func TestValidateLogin(t *testing.T) {
	valid := []string{"admin", "user.name", "user-name", "user_name", "abc123"}
	for _, login := range valid {
		if err := ValidateLogin(login); err != nil {
			t.Errorf("ValidateLogin(%q) = %v, want nil", login, err)
		}
	}
	invalid := []string{"ab", "with space", "кириллица", strings.Repeat("a", 33), "bad/slash"}
	for _, login := range invalid {
		if err := ValidateLogin(login); err == nil {
			t.Errorf("ValidateLogin(%q) = nil, want error", login)
		}
	}
}

func TestTokenHashes(t *testing.T) {
	raw, hash, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if !strings.HasPrefix(raw, "dk_") {
		t.Errorf("token prefix = %q", raw)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	if TokenHash(raw) != hash {
		t.Error("token hash mismatch")
	}
	other, _, _ := NewToken()
	if raw == other {
		t.Error("tokens must be unique")
	}
}

func TestSessionIDs(t *testing.T) {
	first, err := SessionID()
	if err != nil {
		t.Fatalf("SessionID: %v", err)
	}
	second, _ := SessionID()
	if first == second {
		t.Error("session IDs must be unique")
	}
	if HashSessionID(first) == first {
		t.Error("session ID must be hashed for storage")
	}
	if len(HashSessionID(first)) != 64 {
		t.Error("session hash must be sha256 hex")
	}
}

func TestParseUserAgent(t *testing.T) {
	browser, osName := ParseUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36")
	if browser != "Chrome" || osName != "Windows" {
		t.Fatalf("got %s/%s", browser, osName)
	}
	browser, osName = ParseUserAgent("Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0")
	if browser != "Firefox" || osName != "Linux" {
		t.Fatalf("got %s/%s", browser, osName)
	}
	browser, osName = ParseUserAgent("")
	if browser == "" || osName == "" {
		t.Fatal("fallback labels expected")
	}
}
