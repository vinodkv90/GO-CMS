package auth

import (
	"testing"
	"time"
)

func BenchmarkHashPassword(b *testing.B) {
	// Use lighter parameters for benchmark throughput
	params := &Argon2Params{
		Memory:      16 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = HashPassword("SecurePassword2026!", params)
	}
}

func BenchmarkComparePassword(b *testing.B) {
	params := &Argon2Params{
		Memory:      16 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
	hash, err := HashPassword("SecurePassword2026!", params)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ComparePassword("SecurePassword2026!", hash)
	}
}

func BenchmarkGenerateAccessToken(b *testing.B) {
	user := &User{
		ID:       "bench-user-1",
		Username: "benchuser",
		Email:    "bench@cms.local",
		Roles: []Role{
			{Slug: "editor"},
		},
	}
	secret := "a-very-long-secret-key-for-benchmarks-32-chars"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateAccessToken(user, secret, 1*time.Hour)
	}
}

func BenchmarkValidateAccessToken(b *testing.B) {
	user := &User{
		ID:       "bench-user-1",
		Username: "benchuser",
		Email:    "bench@cms.local",
		Roles: []Role{
			{Slug: "editor"},
		},
	}
	secret := "a-very-long-secret-key-for-benchmarks-32-chars"
	token, err := GenerateAccessToken(user, secret, 1*time.Hour)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateAccessToken(token, secret)
	}
}

