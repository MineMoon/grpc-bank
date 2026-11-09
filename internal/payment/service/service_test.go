package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grpc-bank/internal/payment/domain"
	accountpb "grpc-bank/pkg/api/account"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePaymentRepository struct {
	getFn    func(context.Context, string) (*domain.Transaction, error)
	createFn func(context.Context, *domain.Transaction) error
	updateFn func(context.Context, string, domain.TransactionStatus, string) error

	getCalls    int
	createCalls int
	updateCalls int

	updatedID     string
	updatedStatus domain.TransactionStatus
	updatedMsg    string
}

func (f *fakePaymentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	f.getCalls++
	if f.getFn != nil {
		return f.getFn(ctx, key)
	}
	return nil, domain.ErrTransactionNotFound
}

func (f *fakePaymentRepository) CreatePending(ctx context.Context, tx *domain.Transaction) error {
	f.createCalls++
	if f.createFn != nil {
		return f.createFn(ctx, tx)
	}
	return nil
}

func (f *fakePaymentRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, errorMsg string) error {
	f.updateCalls++
	f.updatedID = id
	f.updatedStatus = status
	f.updatedMsg = errorMsg
	if f.updateFn != nil {
		return f.updateFn(ctx, id, status, errorMsg)
	}
	return nil
}

type fakeAccountClient struct {
	transferResp  *accountpb.ExecuteTransferResponse
	transferErr   error
	transferReq   *accountpb.ExecuteTransferRequest
	transferCalls int
}

func (f *fakeAccountClient) GetAccount(context.Context, *accountpb.GetAccountRequest, ...grpc.CallOption) (*accountpb.GetAccountResponse, error) {
	return nil, errors.New("unexpected GetAccount call")
}

func (f *fakeAccountClient) CreateAccount(context.Context, *accountpb.CreateAccountRequest, ...grpc.CallOption) (*accountpb.CreateAccountResponse, error) {
	return nil, errors.New("unexpected CreateAccount call")
}

func (f *fakeAccountClient) ExecuteTransfer(_ context.Context, req *accountpb.ExecuteTransferRequest, _ ...grpc.CallOption) (*accountpb.ExecuteTransferResponse, error) {
	f.transferCalls++
	f.transferReq = req
	if f.transferErr != nil {
		return nil, f.transferErr
	}
	if f.transferResp == nil {
		return &accountpb.ExecuteTransferResponse{Status: "SUCCESS"}, nil
	}
	return f.transferResp, nil
}

func (f *fakeAccountClient) Deposit(context.Context, *accountpb.DepositRequest, ...grpc.CallOption) (*accountpb.DepositResponse, error) {
	return nil, errors.New("unexpected Deposit call")
}

func TestPaymentServiceProcessPaymentValidation(t *testing.T) {
	amount := decimal.NewFromInt(10)

	tests := []struct {
		name           string
		idempotencyKey string
		fromAccountID  string
		toAccountID    string
		amount         decimal.Decimal
		wantMessage    string
	}{
		{name: "missing idempotency key", fromAccountID: "acc-1", toAccountID: "acc-2", amount: amount, wantMessage: "idempotencyKey is required"},
		{name: "missing sender", idempotencyKey: "key-1", toAccountID: "acc-2", amount: amount, wantMessage: "sender and recipient accounts are required"},
		{name: "missing recipient", idempotencyKey: "key-1", fromAccountID: "acc-1", amount: amount, wantMessage: "sender and recipient accounts are required"},
		{name: "same account", idempotencyKey: "key-1", fromAccountID: "acc-1", toAccountID: "acc-1", amount: amount, wantMessage: "can't equal"},
		{name: "zero amount", idempotencyKey: "key-1", fromAccountID: "acc-1", toAccountID: "acc-2", amount: decimal.Zero, wantMessage: "must be strictly positive"},
		{name: "negative amount", idempotencyKey: "key-1", fromAccountID: "acc-1", toAccountID: "acc-2", amount: decimal.NewFromInt(-1), wantMessage: "must be strictly positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePaymentRepository{}
			client := &fakeAccountClient{}
			svc := NewPaymentService(repo, client)

			tx, err := svc.ProcessPayment(context.Background(), tt.idempotencyKey, tt.fromAccountID, tt.toAccountID, tt.amount)
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("expected error containing %q, got %v", tt.wantMessage, err)
			}
			if tx != nil {
				t.Fatalf("expected nil transaction, got %+v", tx)
			}
			if repo.getCalls != 0 || repo.createCalls != 0 || client.transferCalls != 0 {
				t.Fatalf("expected no downstream calls, got get=%d create=%d transfer=%d", repo.getCalls, repo.createCalls, client.transferCalls)
			}
		})
	}
}

func TestPaymentServiceIdempotency(t *testing.T) {
	amount := decimal.NewFromInt(10)

	t.Run("returns existing successful transaction", func(t *testing.T) {
		existing := &domain.Transaction{ID: "tx-1", IdempotencyKey: "key-1", Status: domain.StatusSuccess}
		repo := &fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return existing, nil
			},
		}
		client := &fakeAccountClient{}

		tx, err := NewPaymentService(repo, client).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx != existing {
			t.Fatalf("expected existing transaction %+v, got %+v", existing, tx)
		}
		if repo.createCalls != 0 || client.transferCalls != 0 {
			t.Fatalf("expected no new payment, got create=%d transfer=%d", repo.createCalls, client.transferCalls)
		}
	})

	t.Run("rejects transaction already in progress", func(t *testing.T) {
		repo := &fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return &domain.Transaction{Status: domain.StatusPending}, nil
			},
		}

		_, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "already being processed") {
			t.Fatalf("expected pending error, got %v", err)
		}
		if repo.createCalls != 0 {
			t.Fatalf("expected no new payment, got %d calls", repo.createCalls)
		}
	})

	t.Run("rejects previously failed transaction", func(t *testing.T) {
		repo := &fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return &domain.Transaction{Status: domain.StatusFailed}, nil
			},
		}

		_, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "previous payment attempt") {
			t.Fatalf("expected failed-attempt error, got %v", err)
		}
		if repo.createCalls != 0 {
			t.Fatalf("expected no new payment, got %d calls", repo.createCalls)
		}
	})

	t.Run("propagates idempotency lookup failure", func(t *testing.T) {
		repo := &fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return nil, errors.New("database is down")
			},
		}

		_, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "idempotency check error") {
			t.Fatalf("expected idempotency check error, got %v", err)
		}
		if repo.createCalls != 0 {
			t.Fatalf("expected no new payment, got %d calls", repo.createCalls)
		}
	})

	t.Run("maps duplicate transaction on create", func(t *testing.T) {
		repo := &fakePaymentRepository{
			createFn: func(context.Context, *domain.Transaction) error {
				return domain.ErrDuplicateTransaction
			},
		}

		_, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "already been recorded") {
			t.Fatalf("expected duplicate transaction error, got %v", err)
		}
	})

	t.Run("maps pending create failure", func(t *testing.T) {
		repo := &fakePaymentRepository{
			createFn: func(context.Context, *domain.Transaction) error {
				return errors.New("insert failed")
			},
		}

		_, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "failed to register transaction") {
			t.Fatalf("expected register failure, got %v", err)
		}
	})
}

func TestPaymentServiceTransferFailure(t *testing.T) {
	amount := decimal.RequireFromString("10.5")

	tests := []struct {
		name        string
		transferErr error
		wantMessage string
	}{
		{name: "insufficient funds", transferErr: status.Error(codes.FailedPrecondition, "not enough money"), wantMessage: "Insufficient funds in the account"},
		{name: "missing account", transferErr: status.Error(codes.NotFound, "account not found"), wantMessage: "One of the accounts was not found."},
		{name: "other grpc error", transferErr: status.Error(codes.Internal, "core exploded"), wantMessage: "Banking core error: core exploded"},
		{name: "non grpc error", transferErr: errors.New("connection refused"), wantMessage: "Network communication failure: connection refused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePaymentRepository{
				createFn: func(_ context.Context, tx *domain.Transaction) error {
					tx.ID = "tx-1"
					return nil
				},
			}
			client := &fakeAccountClient{transferErr: tt.transferErr}
			svc := NewPaymentService(repo, client)

			tx, err := svc.ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
			if err == nil || err.Error() != tt.wantMessage {
				t.Fatalf("expected error %q, got %v", tt.wantMessage, err)
			}
			if tx == nil || tx.Status != domain.StatusFailed || tx.ErrorMessage != tt.wantMessage {
				t.Fatalf("expected failed transaction with message %q, got %+v", tt.wantMessage, tx)
			}
			if repo.updateCalls != 1 || repo.updatedID != "tx-1" {
				t.Fatalf("expected one status update for tx-1, got calls=%d id=%q", repo.updateCalls, repo.updatedID)
			}
			if repo.updatedStatus != domain.StatusFailed || repo.updatedMsg != tt.wantMessage {
				t.Fatalf("unexpected status update: status=%q message=%q", repo.updatedStatus, repo.updatedMsg)
			}
			if client.transferCalls != 1 {
				t.Fatalf("expected one transfer call, got %d", client.transferCalls)
			}
		})
	}

	t.Run("ignores status update failure on transfer failure", func(t *testing.T) {
		repo := &fakePaymentRepository{
			createFn: func(_ context.Context, tx *domain.Transaction) error {
				tx.ID = "tx-1"
				return nil
			},
			updateFn: func(context.Context, string, domain.TransactionStatus, string) error {
				return errors.New("update failed")
			},
		}
		client := &fakeAccountClient{transferErr: status.Error(codes.FailedPrecondition, "not enough money")}

		tx, err := NewPaymentService(repo, client).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || err.Error() != "Insufficient funds in the account" {
			t.Fatalf("expected original transfer error, got %v", err)
		}
		if tx == nil || tx.Status != domain.StatusFailed {
			t.Fatalf("expected failed transaction, got %+v", tx)
		}
	})
}

func TestPaymentServiceSuccess(t *testing.T) {
	amount := decimal.RequireFromString("10.5")
	createdAt := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)

	t.Run("records successful transaction", func(t *testing.T) {
		repo := &fakePaymentRepository{
			createFn: func(_ context.Context, tx *domain.Transaction) error {
				tx.ID = "tx-1"
				tx.CreatedAt = createdAt
				return nil
			},
		}
		client := &fakeAccountClient{}

		tx, err := NewPaymentService(repo, client).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tx.ID != "tx-1" || tx.Status != domain.StatusSuccess || !tx.CreatedAt.Equal(createdAt) {
			t.Fatalf("unexpected transaction: %+v", tx)
		}
		if repo.createCalls != 1 || repo.updateCalls != 1 {
			t.Fatalf("expected one create and one update, got create=%d update=%d", repo.createCalls, repo.updateCalls)
		}
		if repo.updatedID != "tx-1" || repo.updatedStatus != domain.StatusSuccess || repo.updatedMsg != "" {
			t.Fatalf("unexpected status update: id=%q status=%q message=%q", repo.updatedID, repo.updatedStatus, repo.updatedMsg)
		}
		if client.transferCalls != 1 {
			t.Fatalf("expected one transfer call, got %d", client.transferCalls)
		}
		if client.transferReq.GetFromAccountId() != "acc-1" || client.transferReq.GetToAccountId() != "acc-2" {
			t.Fatalf("unexpected transfer request: %+v", client.transferReq)
		}
		if client.transferReq.GetAmount() != amount.String() {
			t.Fatalf("expected amount %q, got %q", amount.String(), client.transferReq.GetAmount())
		}
	})

	t.Run("reports status update failure after transfer", func(t *testing.T) {
		repo := &fakePaymentRepository{
			createFn: func(_ context.Context, tx *domain.Transaction) error {
				tx.ID = "tx-1"
				return nil
			},
			updateFn: func(context.Context, string, domain.TransactionStatus, string) error {
				return errors.New("update failed")
			},
		}

		tx, err := NewPaymentService(repo, &fakeAccountClient{}).ProcessPayment(context.Background(), "key-1", "acc-1", "acc-2", amount)
		if err == nil || !strings.Contains(err.Error(), "обновления статуса") {
			t.Fatalf("expected status update error, got %v", err)
		}
		if tx != nil {
			t.Fatalf("expected nil transaction, got %+v", tx)
		}
	})
}
