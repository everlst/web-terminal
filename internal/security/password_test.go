package security

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !valid {
		t.Fatalf("expected password to verify, valid=%v err=%v", valid, err)
	}
	valid, err = VerifyPassword(hash, "incorrect password")
	if err != nil || valid {
		t.Fatalf("expected password to fail, valid=%v err=%v", valid, err)
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}
