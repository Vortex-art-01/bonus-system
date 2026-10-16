package accrual

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

func newServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, WithHTTPClient(srv.Client()))
}

func TestClient_GetOrder_Processed(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/orders/12345678903", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":729.98}`))
	})

	info, err := c.GetOrder(context.Background(), "12345678903")
	require.NoError(t, err)
	assert.Equal(t, &OrderInfo{Order: "12345678903", Status: StatusProcessed, Accrual: model.Money(72998)}, info)
}

func TestClient_GetOrder_WithoutAccrual(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"order":"1","status":"PROCESSING"}`))
	})

	info, err := c.GetOrder(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, info.Status)
	assert.Zero(t, info.Accrual)
}

func TestClient_GetOrder_NotRegistered(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	info, err := c.GetOrder(context.Background(), "1")
	assert.ErrorIs(t, err, ErrOrderNotRegistered)
	assert.Nil(t, info)
}

func TestClient_GetOrder_TooManyRequests(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		{name: "seconds", retryAfter: "60", want: time.Minute},
		{name: "missing header", retryAfter: "", want: DefaultRetryAfter},
		{name: "garbage header", retryAfter: "soon", want: DefaultRetryAfter},
		{name: "non-positive seconds", retryAfter: "0", want: DefaultRetryAfter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("No more than N requests per minute allowed"))
			})

			_, err := c.GetOrder(context.Background(), "1")
			var tooMany *TooManyRequestsError
			require.ErrorAs(t, err, &tooMany)
			assert.Equal(t, tt.want, tooMany.RetryAfter)
			assert.Contains(t, err.Error(), "rate limit")
		})
	}
}

func TestClient_GetOrder_RetryAfterHTTPDate(t *testing.T) {
	at := time.Now().Add(30 * time.Second).UTC()
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", at.Format(http.TimeFormat))
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.GetOrder(context.Background(), "1")
	var tooMany *TooManyRequestsError
	require.ErrorAs(t, err, &tooMany)
	assert.InDelta(t, 30*time.Second, tooMany.RetryAfter, float64(5*time.Second))

	past := time.Now().Add(-time.Minute).UTC()
	assert.Equal(t, DefaultRetryAfter, parseRetryAfter(past.Format(http.TimeFormat)))
}

func TestClient_GetOrder_UnexpectedStatus(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.GetOrder(context.Background(), "1")
	var unexpected *UnexpectedStatusError
	require.ErrorAs(t, err, &unexpected)
	assert.Equal(t, http.StatusInternalServerError, unexpected.StatusCode)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_GetOrder_BadJSON(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"order":`))
	})

	_, err := c.GetOrder(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestClient_GetOrder_EscapesNumber(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/orders/a%2Fb", r.URL.RawPath)
		w.WriteHeader(http.StatusNoContent)
	})

	_, err := c.GetOrder(context.Background(), "a/b")
	assert.ErrorIs(t, err, ErrOrderNotRegistered)
}

func TestClient_GetOrder_ContextCanceled(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.GetOrder(ctx, "1")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestClient_GetOrder_ConnectionError(t *testing.T) {
	c := New("http://127.0.0.1:1", WithHTTPClient(&http.Client{Timeout: time.Second}))

	_, err := c.GetOrder(context.Background(), "1")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrOrderNotRegistered))
}
