package luhn

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		name   string
		number string
		want   bool
	}{
		{name: "spec example", number: "12345678903", want: true},
		{name: "spec order 9278923470", number: "9278923470", want: true},
		{name: "spec order 346436439", number: "346436439", want: true},
		{name: "spec withdrawal order", number: "2377225624", want: true},
		{name: "classic valid", number: "79927398713", want: true},
		{name: "single zero", number: "0", want: true},
		{name: "classic invalid", number: "79927398710", want: false},
		{name: "off by one", number: "12345678904", want: false},
		{name: "empty", number: "", want: false},
		{name: "letters", number: "1234567890a", want: false},
		{name: "spaces inside", number: "1234 5678 903", want: false},
		{name: "negative sign", number: "-12345678903", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.number); got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.number, got, tt.want)
			}
		})
	}
}
