package payment

// Types here mirror `payment_service_spec.md`.

type InitiateRequest struct {
	Amount           float64 `json:"amount"`
	Reason           string  `json:"reason"`
	PayerUserID      string  `json:"payer_user_id"`
	ReceiverID       string  `json:"receiver_id,omitempty"`
	SenderWalletID   string  `json:"sender_wallet_id"`
	ReceiverWalletID string  `json:"receiver_wallet_id,omitempty"`
	ReceiverFullName string  `json:"receiver_full_name,omitempty"`
	TripID           string  `json:"trip_id,omitempty"`
	Message          string  `json:"message,omitempty"`
	PhoneNumber      string  `json:"phone_number"`
	Email            string  `json:"email,omitempty"`
	FirstName        string  `json:"first_name,omitempty"`
	LastName         string  `json:"last_name,omitempty"`
}

type InitiateTopupResponse struct {
	TransactionID string `json:"transaction_id"`
	CheckoutURL   string `json:"checkout_url"`
}

type InitiateInternalResponse struct {
	TransactionID string `json:"transaction_id"`
	TxRef         string `json:"tx_ref"`
}

type TransferRequest struct {
	Amount           float64 `json:"amount"`
	PayerUserID      string  `json:"payer_user_id"`
	SenderWalletID   string  `json:"sender_wallet_id"`
	ReceiverWalletID string  `json:"receiver_wallet_id"`
	ReceiverID       string  `json:"receiver_id"`
	ReceiverFullName string  `json:"receiver_full_name"`
	TripID           string  `json:"trip_id"`
	Message          string  `json:"message,omitempty"`
}

type TransferResponse struct {
	TransactionID string  `json:"transaction_id"`
	TxRef         string  `json:"tx_ref"`
	ReceiptURL    *string `json:"receipt_url"`
}

type TransactionsListResponse struct {
	Items  []map[string]any `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Sort   string           `json:"sort"`
	Order  string           `json:"order"`
}
