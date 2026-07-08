package main

import (
	"testing"
	"time"
)

func TestGetEnvFloat(t *testing.T) {
	tests := []struct {
		name  string
		value string
		def   float64
		want  float64
	}{
		{"unset returns default", "", 100, 100},
		{"valid float", "2.5", 100, 2.5},
		{"valid integer string", "50", 100, 50},
		{"negative", "-1.5", 100, -1.5},
		{"invalid returns default", "abc", 100, 100},
		{"partial garbage returns default", "1.5x", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv("TEST_FLOAT", tt.value)
			}
			if got := getEnvFloat("TEST_FLOAT", tt.def); got != tt.want {
				t.Errorf("getEnvFloat(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name  string
		value string
		def   int
		want  int
	}{
		{"unset returns default", "", 200, 200},
		{"valid int", "42", 200, 42},
		{"negative", "-5", 200, -5},
		{"zero", "0", 200, 0},
		{"invalid returns default", "abc", 200, 200},
		{"mixed digits and letters returns default", "12ab34", 200, 200},
		{"overflow returns default", "99999999999999999999", 200, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv("TEST_INT", tt.value)
			}
			if got := getEnvInt("TEST_INT", tt.def); got != tt.want {
				t.Errorf("getEnvInt(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt64(t *testing.T) {
	tests := []struct {
		name  string
		value string
		def   int64
		want  int64
	}{
		{"unset returns default", "", 1024, 1024},
		{"valid int64", "10485760", 1024, 10485760},
		{"negative", "-1", 1024, -1},
		{"zero", "0", 1024, 0},
		{"invalid returns default", "10MB", 1024, 1024},
		{"overflow returns default", "99999999999999999999", 1024, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv("TEST_INT64", tt.value)
			}
			if got := getEnvInt64("TEST_INT64", tt.def); got != tt.want {
				t.Errorf("getEnvInt64(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name  string
		value string
		def   time.Duration
		want  time.Duration
	}{
		{"unset returns default", "", time.Hour, time.Hour},
		{"valid duration", "30s", time.Hour, 30 * time.Second},
		{"invalid returns default", "soon", time.Hour, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv("TEST_DURATION", tt.value)
			}
			if got := getEnvDuration("TEST_DURATION", tt.def); got != tt.want {
				t.Errorf("getEnvDuration(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
