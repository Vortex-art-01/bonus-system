package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeParser struct{}

func (fakeParser) Parse(token string) (int64, error) {
	if token == "valid" {
		return 42, nil
	}
	return 0, errors.New("bad token")
}

func TestAuth(t *testing.T) {
	var gotUserID int64
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, gotOK = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := Auth(fakeParser{})(next)

	tests := []struct {
		name       string
		prepare    func(r *http.Request)
		wantStatus int
		wantUser   int64
	}{
		{name: "bearer header", prepare: func(r *http.Request) { r.Header.Set("Authorization", "Bearer valid") }, wantStatus: 200, wantUser: 42},
		{name: "lowercase bearer", prepare: func(r *http.Request) { r.Header.Set("Authorization", "bearer valid") }, wantStatus: 200, wantUser: 42},
		{name: "bare header", prepare: func(r *http.Request) { r.Header.Set("Authorization", "valid") }, wantStatus: 200, wantUser: 42},
		{name: "cookie", prepare: func(r *http.Request) { r.AddCookie(&http.Cookie{Name: CookieName, Value: "valid"}) }, wantStatus: 200, wantUser: 42},
		{name: "header wins over cookie", prepare: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer bad")
			r.AddCookie(&http.Cookie{Name: CookieName, Value: "valid"})
		}, wantStatus: 401},
		{name: "missing", prepare: func(*http.Request) {}, wantStatus: 401},
		{name: "invalid header", prepare: func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, wantStatus: 401},
		{name: "invalid cookie", prepare: func(r *http.Request) { r.AddCookie(&http.Cookie{Name: CookieName, Value: "nope"}) }, wantStatus: 401},
		{name: "empty bearer", prepare: func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") }, wantStatus: 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, gotOK = 0, false
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.prepare(r)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, r)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				assert.True(t, gotOK)
				assert.Equal(t, tt.wantUser, gotUserID)
			} else {
				assert.False(t, gotOK)
			}
		})
	}
}

func TestUserIDFromContext(t *testing.T) {
	_, ok := UserIDFromContext(context.Background())
	assert.False(t, ok)

	id, ok := UserIDFromContext(WithUserID(context.Background(), 7))
	assert.True(t, ok)
	assert.Equal(t, int64(7), id)
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func gunzip(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	require.NoError(t, err)
	out, err := io.ReadAll(zr)
	require.NoError(t, err)
	return string(out)
}

func echoHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

func TestGzip_DecompressesRequest(t *testing.T) {
	handler := Gzip(echoHandler(http.StatusOK))
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(gzipBytes(t, "hello")))
	r.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
	assert.Empty(t, w.Header().Get("Content-Encoding"))
}

func TestGzip_MalformedRequestBody(t *testing.T) {
	handler := Gzip(echoHandler(http.StatusOK))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not gzip"))
	r.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGzip_CompressesResponse(t *testing.T) {
	handler := Gzip(echoHandler(http.StatusCreated))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("payload"))
	r.Header.Set("Accept-Encoding", "gzip, deflate")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
	assert.Equal(t, "payload", gunzip(t, w.Body.Bytes()))
}

func TestGzip_ImplicitStatusOK(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("no explicit header"))
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "no explicit header", gunzip(t, w.Body.Bytes()))
}

func TestGzip_NotAccepted(t *testing.T) {
	handler := Gzip(echoHandler(http.StatusOK))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("payload"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, "payload", w.Body.String())
}

func TestGzip_BodilessResponses(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified, http.StatusAccepted} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, r)

			assert.Equal(t, status, w.Code)
			assert.Empty(t, w.Header().Get("Content-Encoding"))
			assert.Empty(t, w.Body.Bytes())
		})
	}
}

func TestGzip_NoContentWithWrite(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		_, _ = w.Write([]byte("ignored by clients"))
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
}

func TestGzip_HandlerWritesNothing(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Empty(t, w.Body.Bytes())
}

func TestGzip_SecondWriteHeaderIgnored(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("tea"))
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "tea", gunzip(t, w.Body.Bytes()))
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))

	r := httptest.NewRequest(http.MethodPost, "/api/user/orders", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusAccepted, w.Code)
	out := buf.String()
	assert.Contains(t, out, "method=POST")
	assert.Contains(t, out, "path=/api/user/orders")
	assert.Contains(t, out, "status=202")
	assert.Contains(t, out, "bytes=2")
	assert.Contains(t, out, "duration=")
}
