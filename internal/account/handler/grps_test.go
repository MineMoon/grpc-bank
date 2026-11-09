package handler

import (
	"context"
	"errors"
	"testing"

	"grpc-bank/internal/account/domain"
	"grpc-bank/internal/account/service"
	accountpb "grpc-bank/pkg/api/account"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAccountRepository struct {
	getFn      func(context.Context, string) (*domain.Account, error)
	createFn   func(context.Context, *domain.Account) error
	transferFn func(context.Context, string, string, decimal.Decimal) (decimal.Decimal, error)
	depositFn  func(context.Context, string, decimal.Decimal) (decimal.Decimal, error)
}

func (f *fakeAccountRepository) GetbyID(ctx context.Context, id string) (*domain.Account, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeAccountRepository) Create(ctx context.Context, acc *domain.Account) error {
	if f.createFn != nil {
		return f.createFn(ctx, acc)
	}
	return nil
}

func (f *fakeAccountRepository) Transfer(ctx context.Context, fromID, toID string, amount decimal.Decimal) (decimal.Decimal, error) {
	if f.transferFn != nil {
		return f.transferFn(ctx, fromID, toID, amount)
	}
	return decimal.Zero, nil
}

func (f *fakeAccountRepository) Deposit(ctx context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
	if f.depositFn != nil {
		return f.depositFn(ctx, accountID, amount)
	}
	return decimal.Zero, nil
}

func newAccountHandler(repo *fakeAccountRepository) *AccountHandler {
	return NewAccountHandler(service.NewAccountService(repo))
}

func TestAccountHandlerGetAccount(t *testing.T) {
	t.Run("rejects missing account id", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{})
		_, err := h.GetAccount(context.Background(), &accountpb.GetAccountRequest{})

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("maps missing account to NotFound", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			getFn: func(context.Context, string) (*domain.Account, error) {
				return nil, domain.ErrAccountNotFound
			},
		})
		_, err := h.GetAccount(context.Background(), &accountpb.GetAccountRequest{AccountId: "acc-1"})

		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("maps repository failure to Internal", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			getFn: func(context.Context, string) (*domain.Account, error) {
				return nil, errors.New("database is down")
			},
		})
		_, err := h.GetAccount(context.Background(), &accountpb.GetAccountRequest{AccountId: "acc-1"})

		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("returns account details", func(t *testing.T) {
		balance := decimal.RequireFromString("123.45")
		h := newAccountHandler(&fakeAccountRepository{
			getFn: func(_ context.Context, id string) (*domain.Account, error) {
				if id != "acc-1" {
					t.Fatalf("unexpected repository id %q", id)
				}
				return &domain.Account{
					ID:            "acc-1",
					UserID:        "user-1",
					AccountNumber: "40817810000000000001",
					Balance:       balance,
					Currency:      "RUB",
				}, nil
			},
		})

		resp, err := h.GetAccount(context.Background(), &accountpb.GetAccountRequest{AccountId: "acc-1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.AccountId != "acc-1" || resp.UserId != "user-1" {
			t.Fatalf("unexpected response: %+v", resp)
		}
		if resp.AccountNumber != "40817810000000000001" || resp.Currency != "RUB" {
			t.Fatalf("unexpected response: %+v", resp)
		}
		if resp.Balance != balance.String() {
			t.Fatalf("expected balance %q, got %q", balance.String(), resp.Balance)
		}
	})
}

func TestAccountHandlerCreateAccount(t *testing.T) {
	t.Run("rejects missing user id", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{})
		_, err := h.CreateAccount(context.Background(), &accountpb.CreateAccountRequest{})

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("maps duplicate account to AlreadyExists", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			createFn: func(context.Context, *domain.Account) error {
				return domain.ErrAccountAlreadyExists
			},
		})
		_, err := h.CreateAccount(context.Background(), &accountpb.CreateAccountRequest{UserId: "user-1"})

		if status.Code(err) != codes.AlreadyExists {
			t.Fatalf("expected AlreadyExists, got %v", err)
		}
	})

	t.Run("maps repository failure to Internal", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			createFn: func(context.Context, *domain.Account) error {
				return errors.New("insert failed")
			},
		})
		_, err := h.CreateAccount(context.Background(), &accountpb.CreateAccountRequest{UserId: "user-1"})

		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("returns created account", func(t *testing.T) {
		var createdCurrency string
		h := newAccountHandler(&fakeAccountRepository{
			createFn: func(_ context.Context, acc *domain.Account) error {
				createdCurrency = acc.Currency
				acc.ID = "acc-1"
				return nil
			},
		})

		resp, err := h.CreateAccount(context.Background(), &accountpb.CreateAccountRequest{UserId: "user-1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createdCurrency != "RUB" {
			t.Fatalf("expected default currency RUB, got %q", createdCurrency)
		}
		if resp.AccountId != "acc-1" {
			t.Fatalf("expected account id %q, got %q", "acc-1", resp.AccountId)
		}
		if resp.AccountNumber == "" {
			t.Fatal("expected generated account number")
		}
	})
}

func TestAccountHandlerExecuteTransfer(t *testing.T) {
	t.Run("rejects missing account ids", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			req  *accountpb.ExecuteTransferRequest
		}{
			{name: "missing from", req: &accountpb.ExecuteTransferRequest{ToAccountId: "acc-2", Amount: "10"}},
			{name: "missing to", req: &accountpb.ExecuteTransferRequest{FromAccountId: "acc-1", Amount: "10"}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				h := newAccountHandler(&fakeAccountRepository{})
				_, err := h.ExecuteTransfer(context.Background(), tt.req)

				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("expected InvalidArgument, got %v", err)
				}
			})
		}
	})

	t.Run("rejects invalid amount", func(t *testing.T) {
		for _, amount := range []string{"", "abc", "0", "-1"} {
			t.Run("amount "+amount, func(t *testing.T) {
				h := newAccountHandler(&fakeAccountRepository{})
				_, err := h.ExecuteTransfer(context.Background(), &accountpb.ExecuteTransferRequest{
					FromAccountId: "acc-1",
					ToAccountId:   "acc-2",
					Amount:        amount,
				})

				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("expected InvalidArgument, got %v", err)
				}
			})
		}
	})

	for _, tt := range []struct {
		name     string
		repoErr  error
		wantCode codes.Code
	}{
		{name: "missing account", repoErr: domain.ErrAccountNotFound, wantCode: codes.NotFound},
		{name: "insufficient funds", repoErr: domain.ErrInsufficientFunds, wantCode: codes.FailedPrecondition},
		{name: "same account", repoErr: domain.ErrSameAccountTransfer, wantCode: codes.InvalidArgument},
		{name: "repository failure", repoErr: errors.New("commit failed"), wantCode: codes.Internal},
	} {
		t.Run("maps "+tt.name, func(t *testing.T) {
			h := newAccountHandler(&fakeAccountRepository{
				transferFn: func(context.Context, string, string, decimal.Decimal) (decimal.Decimal, error) {
					return decimal.Zero, tt.repoErr
				},
			})
			_, err := h.ExecuteTransfer(context.Background(), &accountpb.ExecuteTransferRequest{
				FromAccountId: "acc-1",
				ToAccountId:   "acc-2",
				Amount:        "10.5",
			})

			if status.Code(err) != tt.wantCode {
				t.Fatalf("expected %v, got %v", tt.wantCode, err)
			}
		})
	}

	t.Run("returns new balance", func(t *testing.T) {
		wantBalance := decimal.RequireFromString("89.5")
		h := newAccountHandler(&fakeAccountRepository{
			transferFn: func(_ context.Context, fromID, toID string, amount decimal.Decimal) (decimal.Decimal, error) {
				if fromID != "acc-1" || toID != "acc-2" {
					t.Fatalf("unexpected accounts: from %q to %q", fromID, toID)
				}
				if !amount.Equal(decimal.RequireFromString("10.5")) {
					t.Fatalf("unexpected amount %s", amount)
				}
				return wantBalance, nil
			},
		})

		resp, err := h.ExecuteTransfer(context.Background(), &accountpb.ExecuteTransferRequest{
			FromAccountId: "acc-1",
			ToAccountId:   "acc-2",
			Amount:        "10.5",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "SUCCESS" {
			t.Fatalf("expected SUCCESS, got %q", resp.Status)
		}
		if resp.FromBalanceAfter != wantBalance.String() {
			t.Fatalf("expected balance %q, got %q", wantBalance.String(), resp.FromBalanceAfter)
		}
	})
}

func TestAccountHandlerDeposit(t *testing.T) {
	t.Run("rejects invalid amount", func(t *testing.T) {
		for _, amount := range []string{"", "abc", "0", "-1"} {
			t.Run("amount "+amount, func(t *testing.T) {
				h := newAccountHandler(&fakeAccountRepository{})
				_, err := h.Deposit(context.Background(), &accountpb.DepositRequest{
					AccountId: "acc-1",
					Amount:    amount,
				})

				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("expected InvalidArgument, got %v", err)
				}
			})
		}
	})

	t.Run("maps missing account to NotFound", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			depositFn: func(context.Context, string, decimal.Decimal) (decimal.Decimal, error) {
				return decimal.Zero, domain.ErrAccountNotFound
			},
		})
		_, err := h.Deposit(context.Background(), &accountpb.DepositRequest{AccountId: "acc-1", Amount: "10"})

		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})

	t.Run("maps repository failure to Internal", func(t *testing.T) {
		h := newAccountHandler(&fakeAccountRepository{
			depositFn: func(context.Context, string, decimal.Decimal) (decimal.Decimal, error) {
				return decimal.Zero, errors.New("update failed")
			},
		})
		_, err := h.Deposit(context.Background(), &accountpb.DepositRequest{AccountId: "acc-1", Amount: "10"})

		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("returns new balance", func(t *testing.T) {
		wantBalance := decimal.RequireFromString("110")
		h := newAccountHandler(&fakeAccountRepository{
			depositFn: func(_ context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
				if accountID != "acc-1" {
					t.Fatalf("unexpected account id %q", accountID)
				}
				if !amount.Equal(decimal.NewFromInt(10)) {
					t.Fatalf("unexpected amount %s", amount)
				}
				return wantBalance, nil
			},
		})

		resp, err := h.Deposit(context.Background(), &accountpb.DepositRequest{AccountId: "acc-1", Amount: "10"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != "SUCCESS" {
			t.Fatalf("expected SUCCESS, got %q", resp.Status)
		}
		if resp.BalanceAfter != wantBalance.String() {
			t.Fatalf("expected balance %q, got %q", wantBalance.String(), resp.BalanceAfter)
		}
	})
}
