package model

import "testing"

func TestOrderStatus_IsFinal(t *testing.T) {
	tests := map[OrderStatus]bool{
		OrderStatusNew:        false,
		OrderStatusProcessing: false,
		OrderStatusInvalid:    true,
		OrderStatusProcessed:  true,
		OrderStatus("WEIRD"):  false,
	}
	for status, want := range tests {
		if got := status.IsFinal(); got != want {
			t.Errorf("%s.IsFinal() = %v, want %v", status, got, want)
		}
	}
}
