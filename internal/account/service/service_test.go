package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"grpc-bank/internal/account/domain"

	"github.com/shopspring/decimal"
)

type fakeAccountRepository struct {
	getAccount *domain.Account
	getErr     error
	getID      string
	getCalls   int

	createErr   error
	createCalls int

	transferBalance decimal.Decimal
	transferErr     error
	transferFrom    string
	transferTo      string
	transferAmount  decimal.Decimal
	transferCalls   int

	depositBalance decimal.Decimal
	depositErr     error
	depositID      string
	depositAmount  decimal.Decimal
	depositCalls   int
}

func (f *fakeAccountRepository) GetbyID(_ context.Context, id string) (*domain.Account, error) {
	f.getCalls++
	f.getID = id
	return f.getAccount, f.getErr
}

func (f *fakeAccountRepository) Create(_ context.Context, acc *domain.Account) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	acc.ID = "acc-1"
	return nil
}

func (f *fakeAccountRepository) Transfer(_ context.Context, fromID, toID string, amount decimal.Decimal) (decimal.Decimal, error) {
	f.transferCalls++
	f.transferFrom = fromID
	f.transferTo = toID
	f.transferAmount = amount
	return f.transferBalance, f.transferErr
}

func (f *fakeAccountRepository) Deposit(_ context.Context, accountID string, amount decimal.Decimal) (decimal.Decimal, error) {
	f.depositCalls++
	f.depositID = accountID
	f.depositAmount = amount
	return f.depositBalance, f.depositErr
}

func TestAccountServiceGetAccount(t *testing.T) {
	t.Run("rejects empty id", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		acc, err := NewAccountService(repo).GetAccount(context.Background(), "")

		if err == nil || !strings.Contains(err.Error(), "can not be empty") {
			t.Fatalf("expected empty id error, got %v", err)
		}
		if acc != nil {
			t.Fatalf("expected nil account, got %+v", acc)
		}
		if repo.getCalls != 0 {
			t.Fatalf("expected repository not to be called, got %d calls", repo.getCalls)
		}
	})

	t.Run("returns repository account", func(t *testing.T) {
		want := &domain.Account{ID: "acc-1", UserID: "user-1", Currency: "RUB"}
		repo := &fakeAccountRepository{getAccount: want}
		got, err := NewAccountService(repo).GetAccount(context.Background(), "acc-1")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("expected %+v, got %+v", want, got)
		}
		if repo.getID != "acc-1" {
			t.Fatalf("expected repository id %q, got %q", "acc-1", repo.getID)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		wantErr := errors.New("database is down")
		repo := &fakeAccountRepository{getErr: wantErr}

		_, err := NewAccountService(repo).GetAccount(context.Background(), "acc-1")
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
	})
}

func TestAccountServiceCreateAccount(t *testing.T) {
	accountNumberPattern := regexp.MustCompile(`^40817810\d{12}$`)

	t.Run("rejects empty user id", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		acc, err := NewAccountService(repo).CreateAccount(context.Background(), "", "RUB")

		if err == nil || !strings.Contains(err.Error(), "user_id can not be empty") {
			t.Fatalf("expected empty user id error, got %v", err)
		}
		if acc != nil {
			t.Fatalf("expected nil account, got %+v", acc)
		}
		if repo.createCalls != 0 {
			t.Fatalf("expected repository not to be called, got %d calls", repo.createCalls)
		}
	})

	t.Run("defaults currency to RUB", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		acc, err := NewAccountService(repo).CreateAccount(context.Background(), "user-1", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if acc.Currency != "RUB" {
			t.Fatalf("expected RUB currency, got %q", acc.Currency)
		}
		if !acc.Balance.Equal(decimal.Zero) {
			t.Fatalf("expected zero balance, got %s", acc.Balance)
		}
		if !accountNumberPattern.MatchString(acc.AccountNumber) {
			t.Fatalf("unexpected account number %q", acc.AccountNumber)
		}
		if acc.ID != "acc-1" {
			t.Fatalf("expected repository-assigned id, got %q", acc.ID)
		}
	})

	t.Run("keeps explicit currency", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		acc, err := NewAccountService(repo).CreateAccount(context.Background(), "user-1", "USD")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if acc.Currency != "USD" {
			t.Fatalf("expected USD currency, got %q", acc.Currency)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		wantErr := domain.ErrAccountAlreadyExists
		repo := &fakeAccountRepository{createErr: wantErr}

		acc, err := NewAccountService(repo).CreateAccount(context.Background(), "user-1", "RUB")
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
		if acc != nil {
			t.Fatalf("expected nil account, got %+v", acc)
		}
	})
}

func TestAccountServiceTransfer(t *testing.T) {
	amount := decimal.RequireFromString("10.5")

	t.Run("rejects same account", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		balance, err := NewAccountService(repo).Transfer(context.Background(), "acc-1", "acc-1", amount)

		if !errors.Is(err, domain.ErrSameAccountTransfer) {
			t.Fatalf("expected %v, got %v", domain.ErrSameAccountTransfer, err)
		}
		if !balance.Equal(decimal.Zero) {
			t.Fatalf("expected zero balance, got %s", balance)
		}
		if repo.transferCalls != 0 {
			t.Fatalf("expected repository not to be called, got %d calls", repo.transferCalls)
		}
	})

	for _, tt := range []struct {
		name   string
		amount decimal.Decimal
	}{
		{name: "zero amount", amount: decimal.Zero},
		{name: "negative amount", amount: decimal.NewFromInt(-1)},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			repo := &fakeAccountRepository{}
			balance, err := NewAccountService(repo).Transfer(context.Background(), "acc-1", "acc-2", tt.amount)

			if !errors.Is(err, domain.ErrInvalidAmount) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidAmount, err)
			}
			if !balance.Equal(decimal.Zero) {
				t.Fatalf("expected zero balance, got %s", balance)
			}
			if repo.transferCalls != 0 {
				t.Fatalf("expected repository not to be called, got %d calls", repo.transferCalls)
			}
		})
	}

	t.Run("delegates to repository", func(t *testing.T) {
		wantBalance := decimal.NewFromInt(42)
		repo := &fakeAccountRepository{transferBalance: wantBalance}
		balance, err := NewAccountService(repo).Transfer(context.Background(), "acc-1", "acc-2", amount)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !balance.Equal(wantBalance) {
			t.Fatalf("expected balance %s, got %s", wantBalance, balance)
		}
		if repo.transferCalls != 1 {
			t.Fatalf("expected one repository call, got %d", repo.transferCalls)
		}
		if repo.transferFrom != "acc-1" || repo.transferTo != "acc-2" {
			t.Fatalf("unexpected accounts: from %q to %q", repo.transferFrom, repo.transferTo)
		}
		if !repo.transferAmount.Equal(amount) {
			t.Fatalf("expected amount %s, got %s", amount, repo.transferAmount)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		wantErr := domain.ErrInsufficientFunds
		repo := &fakeAccountRepository{transferErr: wantErr}

		_, err := NewAccountService(repo).Transfer(context.Background(), "acc-1", "acc-2", amount)
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
	})
}

func TestAccountServiceDeposit(t *testing.T) {
	amount := decimal.RequireFromString("10.5")

	t.Run("rejects empty account id", func(t *testing.T) {
		repo := &fakeAccountRepository{}
		balance, err := NewAccountService(repo).Deposit(context.Background(), "", amount)

		if err == nil || !strings.Contains(err.Error(), "can not be empty") {
			t.Fatalf("expected empty account id error, got %v", err)
		}
		if !balance.Equal(decimal.Zero) {
			t.Fatalf("expected zero balance, got %s", balance)
		}
		if repo.depositCalls != 0 {
			t.Fatalf("expected repository not to be called, got %d calls", repo.depositCalls)
		}
	})

	for _, tt := range []struct {
		name   string
		amount decimal.Decimal
	}{
		{name: "zero amount", amount: decimal.Zero},
		{name: "negative amount", amount: decimal.NewFromInt(-1)},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			repo := &fakeAccountRepository{}
			balance, err := NewAccountService(repo).Deposit(context.Background(), "acc-1", tt.amount)

			if !errors.Is(err, domain.ErrInvalidAmount) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidAmount, err)
			}
			if !balance.Equal(decimal.Zero) {
				t.Fatalf("expected zero balance, got %s", balance)
			}
			if repo.depositCalls != 0 {
				t.Fatalf("expected repository not to be called, got %d calls", repo.depositCalls)
			}
		})
	}

	t.Run("delegates to repository", func(t *testing.T) {
		wantBalance := decimal.NewFromInt(99)
		repo := &fakeAccountRepository{depositBalance: wantBalance}
		balance, err := NewAccountService(repo).Deposit(context.Background(), "acc-1", amount)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !balance.Equal(wantBalance) {
			t.Fatalf("expected balance %s, got %s", wantBalance, balance)
		}
		if repo.depositCalls != 1 {
			t.Fatalf("expected one repository call, got %d", repo.depositCalls)
		}
		if repo.depositID != "acc-1" {
			t.Fatalf("expected account id %q, got %q", "acc-1", repo.depositID)
		}
		if !repo.depositAmount.Equal(amount) {
			t.Fatalf("expected amount %s, got %s", amount, repo.depositAmount)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		wantErr := domain.ErrAccountNotFound
		repo := &fakeAccountRepository{depositErr: wantErr}

		_, err := NewAccountService(repo).Deposit(context.Background(), "acc-1", amount)
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected %v, got %v", wantErr, err)
		}
	})
}
