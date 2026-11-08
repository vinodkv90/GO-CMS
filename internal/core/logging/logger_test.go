package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_JSON_WithContext(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  "info",
		Format: "json",
	}

	logger := NewLogger(cfg, &buf)
	ctx := WithContext(context.Background(), "req-12345", "tenant-abc", "user-42")

	logger.InfoContext(ctx, "Test message", "action", "create_post")

	var parsed map[string]any
	err := json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "Test message", parsed["msg"])
	assert.Equal(t, "INFO", parsed["level"])
	assert.Equal(t, "create_post", parsed["action"])
	assert.Equal(t, "req-12345", parsed["request_id"])
	assert.Equal(t, "tenant-abc", parsed["tenant_id"])
	assert.Equal(t, "user-42", parsed["user_id"])
}

func TestNewLogger_Text_Format(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  "debug",
		Format: "text",
	}

	logger := NewLogger(cfg, &buf)
	logger.Debug("Debug test", "counter", 99)

	output := buf.String()
	assert.Contains(t, output, "DEBUG")
	assert.Contains(t, output, "Debug test")
	assert.Contains(t, output, "counter=99")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "DEBUG"},
		{"DEBUG", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
	}

	for _, tt := range tests {
		lvl := ParseLevel(tt.input)
		assert.Equal(t, tt.expected, strings.ToUpper(lvl.String()))
	}
}

