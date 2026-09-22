package model

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMoney(t *testing.T) {
	tests := []struct {
		in   string
		want Money
	}{
		{"0", 0},
		{"5", 500},
		{"729.98", 72998},
		{"500.5", 50050},
		{"0.01", 1},
		{"-0.01", -1},
		{"-729.98", -72998},
		{"1e2", 10000},
		{"0.005", 1},   // half away from zero
		{"-0.005", -1}, // half away from zero
		{"0.004", 0},   // rounds down
		{"0.994", 99},  // rounds down
		{"0.996", 100}, // rounds up
		{"1.005", 101}, // half away from zero, unlike float64 arithmetic
		{"92233720368.54", 9223372036854},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMoney(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseMoney_Invalid(t *testing.T) {
	for _, in := range []string{"", "abc", "1,5", "1.2.3", "92233720368547758.08", "-92233720368547758.09"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseMoney(in)
			assert.Error(t, err)
		})
	}
}

func TestMoney_String(t *testing.T) {
	tests := map[Money]string{
		0:      "0",
		1:      "0.01",
		10:     "0.1",
		99:     "0.99",
		100:    "1",
		50050:  "500.5",
		72998:  "729.98",
		-1:     "-0.01",
		-72998: "-729.98",
		-100:   "-1",
	}
	for m, want := range tests {
		assert.Equal(t, want, m.String(), "Money(%d)", int64(m))
	}
}

func TestMoney_StringRoundTripAtExtremes(t *testing.T) {
	for _, want := range []Money{math.MinInt64, math.MaxInt64} {
		got, err := ParseMoney(want.String())
		require.NoError(t, err, "Money(%d) = %q", int64(want), want.String())
		assert.Equal(t, want, got)
	}
}

func TestMoney_JSONRoundTrip(t *testing.T) {
	type payload struct {
		Sum     Money  `json:"sum"`
		Accrual *Money `json:"accrual,omitempty"`
	}

	accrual := Money(50000)
	data, err := json.Marshal(payload{Sum: 72998, Accrual: &accrual})
	require.NoError(t, err)
	assert.JSONEq(t, `{"sum":729.98,"accrual":500}`, string(data))

	var got payload
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, Money(72998), got.Sum)
	require.NotNil(t, got.Accrual)
	assert.Equal(t, accrual, *got.Accrual)
}

func TestMoney_UnmarshalJSON(t *testing.T) {
	t.Run("omitted field keeps zero value", func(t *testing.T) {
		var v struct {
			Sum Money `json:"sum"`
		}
		require.NoError(t, json.Unmarshal([]byte(`{}`), &v))
		assert.Equal(t, Money(0), v.Sum)
	})

	t.Run("null keeps current value", func(t *testing.T) {
		m := Money(42)
		require.NoError(t, json.Unmarshal([]byte(`null`), &m))
		assert.Equal(t, Money(42), m)
	})

	t.Run("more than two decimals is rounded", func(t *testing.T) {
		var m Money
		require.NoError(t, json.Unmarshal([]byte(`729.9849`), &m))
		assert.Equal(t, Money(72998), m)
	})

	t.Run("rejects a string", func(t *testing.T) {
		var m Money
		assert.Error(t, json.Unmarshal([]byte(`"729.98"`), &m))
	})

	t.Run("rejects a non-number", func(t *testing.T) {
		var m Money
		assert.Error(t, json.Unmarshal([]byte(`true`), &m))
	})
}

func TestMoney_ExactArithmetic(t *testing.T) {
	a, b := Money(10), Money(20)
	assert.Equal(t, Money(30), a+b)
	assert.Equal(t, "0.3", (a + b).String())

	var balance Money
	for range 1000 {
		balance += 1
	}
	assert.Equal(t, Money(1000), balance)
	assert.Equal(t, "10", balance.String())
}
