package services

import (
	"context"
	"time"

	"wallet_service/internal/models"
	"wallet_service/internal/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type WalletService struct {
	WalletRepo *repository.WalletRepository
}

type TransferHook func(ctx context.Context) error

// TransferBalance atomically transfers amount from fromWalletID to toWalletID.
// It enforces:
// - amount > 0
// - both wallets exist
// - both wallets are not frozen
// - from wallet remains non-negative
func (s *WalletService) TransferBalance(ctx context.Context, fromWalletID, toWalletID int64, amount decimal.Decimal) error {
	return s.transferBalance(ctx, fromWalletID, toWalletID, amount, nil)
}

// TransferBalanceWithHook performs the same atomic transfer as TransferBalance, but runs hook
// after balances are updated and before the DB transaction commits.
func (s *WalletService) TransferBalanceWithHook(ctx context.Context, fromWalletID, toWalletID int64, amount decimal.Decimal, hook TransferHook) error {
	return s.transferBalance(ctx, fromWalletID, toWalletID, amount, hook)
}

func (s *WalletService) transferBalance(ctx context.Context, fromWalletID, toWalletID int64, amount decimal.Decimal, hook TransferHook) error {
	if amount.Cmp(decimal.Zero) <= 0 {
		return ErrInvalidAmount
	}
	if fromWalletID == toWalletID {
		return ErrSameWalletTransfer
	}

	db := s.WalletRepo.DB()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock in a stable order to reduce deadlock risk.
		firstID, secondID := fromWalletID, toWalletID
		if secondID < firstID {
			firstID, secondID = secondID, firstID
		}

		first, err := s.WalletRepo.LockByID(ctx, tx, firstID)
		if err != nil {
			return err
		}
		second, err := s.WalletRepo.LockByID(ctx, tx, secondID)
		if err != nil {
			return err
		}

		var from, to models.Wallet
		if first.ID == fromWalletID {
			from, to = first, second
		} else {
			to, from = first, second
		}

		if from.Freezed || to.Freezed {
			return ErrWalletFrozen
		}
		if from.Balance.Cmp(amount) < 0 {
			return ErrInsufficientFunds
		}

		now := time.Now().UTC()
		from.Balance = from.Balance.Sub(amount)
		to.Balance = to.Balance.Add(amount)
		from.UpdatedAt = now
		to.UpdatedAt = now

		if err := tx.Save(&from).Error; err != nil {
			return err
		}
		if err := tx.Save(&to).Error; err != nil {
			return err
		}
		if hook != nil {
			if err := hook(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// ApplyTopupIdempotent credits a wallet exactly once for a given payment service transaction id.
// If the transaction was already applied, applied is false and newBalance is the current balance.
func (s *WalletService) ApplyTopupIdempotent(
	ctx context.Context,
	paymentTransactionID string,
	walletID int64,
	amount decimal.Decimal,
	currency string,
	txRef *string,
	chapaReference *string,
) (applied bool, newBalance decimal.Decimal, err error) {
	if amount.Cmp(decimal.Zero) <= 0 {
		return false, decimal.Zero, ErrInvalidAmount
	}

	db := s.WalletRepo.DB()
	var outApplied bool
	var outBal decimal.Decimal
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := s.WalletRepo.LockByID(ctx, tx, walletID)
		if err != nil {
			return err
		}
		if w.Freezed {
			return ErrWalletFrozen
		}

		credit := models.WalletTopupCredit{
			PaymentTransactionID: paymentTransactionID,
			WalletID:             walletID,
			Amount:               amount,
			Currency:             currency,
			TxRef:                txRef,
			ChapaReference:       chapaReference,
		}
		created, err := s.WalletRepo.CreateTopupCreditIfNotExists(ctx, tx, &credit)
		if err != nil {
			return err
		}
		if !created {
			outApplied = false
			outBal = w.Balance
			return nil
		}

		w.Balance = w.Balance.Add(amount)
		w.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&w).Error; err != nil {
			return err
		}
		outApplied = true
		outBal = w.Balance
		return nil
	})
	return outApplied, outBal, txErr
}
