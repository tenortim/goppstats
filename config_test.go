package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestSecretFromEnv(t *testing.T) {
	const envKey = "GOPPSTATS_TEST_SECRET"
	const envVal = "s3cr3t"

	t.Run("literal string returned unchanged", func(t *testing.T) {
		got, err := secretFromEnv("mypassword")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "mypassword" {
			t.Errorf("got %q, want %q", got, "mypassword")
		}
	})

	t.Run("env prefix with set variable", func(t *testing.T) {
		t.Setenv(envKey, envVal)
		got, err := secretFromEnv(envPrefix + envKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != envVal {
			t.Errorf("got %q, want %q", got, envVal)
		}
	})

	t.Run("env prefix with unset variable", func(t *testing.T) {
		os.Unsetenv(envKey)
		_, err := secretFromEnv(envPrefix + envKey)
		if err == nil {
			t.Error("expected error for unset env var, got nil")
		}
	})

	t.Run("empty string returned unchanged", func(t *testing.T) {
		got, err := secretFromEnv("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestValidateConfigVersion_Valid(t *testing.T) {
	valid := []string{"0.29", "0.30", "0.31", "v0.29", "V0.31"}
	for _, v := range valid {
		t.Run(v, func(t *testing.T) {
			// Should not panic or exit
			validateConfigVersion(v)
		})
	}
}

// TestValidateConfigVersion_Invalid uses a subprocess to verify that
// validateConfigVersion calls os.Exit(1) for bad version strings.
func TestValidateConfigVersion_Invalid(t *testing.T) {
	cases := []struct {
		name    string
		version string
		envKey  string
	}{
		{"empty", "", "TEST_DIE_EMPTY"},
		{"unsupported", "0.99", "TEST_DIE_UNSUPPORTED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if os.Getenv(tc.envKey) == "1" {
				// We are the subprocess; exercise the die path and exit
				validateConfigVersion(tc.version)
				return
			}
			cmd := exec.Command(os.Args[0], "-test.run=TestValidateConfigVersion_Invalid/"+tc.name)
			cmd.Env = append(os.Environ(), tc.envKey+"=1")
			err := cmd.Run()
			if e, ok := err.(*exec.ExitError); ok && !e.Success() {
				return // got an exit error — expected
			}
			t.Fatalf("expected non-zero exit for version %q, got: %v", tc.version, err)
		})
	}
}
