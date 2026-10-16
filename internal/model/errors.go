package model

import "errors"

var (
	ErrLoginTaken             = errors.New("login is already taken")
	ErrUserNotFound           = errors.New("user not found")
	ErrInvalidCredentials     = errors.New("invalid login or password")
	ErrInvalidOrderNumber     = errors.New("invalid order number")
	ErrOrderAlreadyUploaded   = errors.New("order was already uploaded by this user")
	ErrOrderUploadedByAnother = errors.New("order was already uploaded by another user")
	ErrInvalidWithdrawalSum   = errors.New("withdrawal sum must be positive")
	ErrInsufficientFunds      = errors.New("insufficient funds")
)
