package repository

import (
	"context"
	"errors"
	"testing"

	"grpc-bank/internal/account/domain"

	"github.com/shopspring/decimal"
)

func TestAccountRepositoryDepositRejectsNonPositiveAmount(t *testing.T) {
	repo := NewAccountReposiroty(nil)

	for _, amount := range []decimal.Decimal{
		decimal.Zero,
		decimal.NewFromInt(-1),
		decimal.RequireFromString("-0.01"),
	} {
		t.Run(amount.String(), func(t *testing.T) {
			balance, err := repo.Deposit(context.Background(), "acc-1", amount)

			if !errors.Is(err, domain.ErrInvalidAmount) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidAmount, err)
			}
			if !balance.Equal(decimal.Zero) {
				t.Fatalf("expected zero balance, got %s", balance)
			}
		})
	}
}
