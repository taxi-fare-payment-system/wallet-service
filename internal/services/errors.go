package services

import "errors"

var (
	ErrWalletFrozen       = errors.New("wallet is frozen")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInvalidAmount      = errors.New("invalid amount")
	ErrSameWalletTransfer = errors.New("cannot transfer to same wallet")
)
