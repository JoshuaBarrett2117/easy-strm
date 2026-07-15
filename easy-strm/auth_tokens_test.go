package main

import "testing"

func TestGenerateAndVerifyToken(t *testing.T) {
	const secret = "unit-test-secret"

	token, err := GenerateToken(123, secret)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := VerifyToken(token, secret)
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if claims.UserID != 123 {
		t.Fatalf("claims.UserID = %d, want 123", claims.UserID)
	}
}

func TestVerifyTokenRejectsWrongSecret(t *testing.T) {
	token, err := GenerateToken(123, "right-secret")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if _, err := VerifyToken(token, "wrong-secret"); err == nil {
		t.Fatalf("VerifyToken() error = nil, want invalid token error")
	}
}
