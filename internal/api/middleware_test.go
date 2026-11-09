package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaxBodySizeMiddlewareByPath(t *testing.T) {
	handler := MaxBodySizeMiddlewareByPath(4, func(path string) int64 {
		if path == "/media/hls" {
			return 8
		}
		return 4
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/media/hls", strings.NewReader("12345678"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	request = httptest.NewRequest(http.MethodPost, "/media", strings.NewReader("12345"))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}
