package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type Status string

const (
	StatusRegistered Status = "REGISTERED"
	StatusInvalid    Status = "INVALID"
	StatusProcessing Status = "PROCESSING"
	StatusProcessed  Status = "PROCESSED"
)

const (
	DefaultTimeout    = 10 * time.Second
	DefaultRetryAfter = time.Minute
	maxResponseSize   = 1 << 20
)

type OrderInfo struct {
	Order   string      `json:"order"`
	Status  Status      `json:"status"`
	Accrual model.Money `json:"accrual"`
}

var ErrOrderNotRegistered = errors.New("order is not registered in the accrual system")

type TooManyRequestsError struct {
	RetryAfter time.Duration
}

func (e *TooManyRequestsError) Error() string {
	return fmt.Sprintf("accrual system rate limit exceeded, retry after %s", e.RetryAfter)
}

type UnexpectedStatusError struct {
	StatusCode int
}

func (e *UnexpectedStatusError) Error() string {
	return fmt.Sprintf("accrual system returned unexpected status %d", e.StatusCode)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: DefaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) GetOrder(ctx context.Context, number string) (*OrderInfo, error) {
	endpoint := c.baseURL + "/api/orders/" + url.PathEscape(number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request accrual system: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var info OrderInfo
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&info); err != nil {
			return nil, fmt.Errorf("decode accrual response: %w", err)
		}
		return &info, nil
	case http.StatusNoContent:
		return nil, ErrOrderNotRegistered
	case http.StatusTooManyRequests:
		return nil, &TooManyRequestsError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	default:
		return nil, &UnexpectedStatusError{StatusCode: resp.StatusCode}
	}
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return DefaultRetryAfter
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return DefaultRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if d := time.Until(at); d > 0 {
			return d
		}
	}
	return DefaultRetryAfter
}
