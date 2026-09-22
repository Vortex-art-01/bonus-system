package model

import "time"

type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "NEW"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

func (s OrderStatus) IsFinal() bool {
	return s == OrderStatusInvalid || s == OrderStatusProcessed
}

type User struct {
	ID           int64
	Login        string
	PasswordHash string
	CreatedAt    time.Time
}

type Order struct {
	Number     string
	UserID     int64
	Status     OrderStatus
	Accrual    *Money
	UploadedAt time.Time
}

type Balance struct {
	Current   Money
	Withdrawn Money
}

type Withdrawal struct {
	ID          int64
	UserID      int64
	Order       string
	Sum         Money
	ProcessedAt time.Time
}
