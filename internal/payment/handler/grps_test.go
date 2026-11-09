package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"grpc-bank/internal/payment/domain"
	"grpc-bank/internal/payment/service"
	accountpb "grpc-bank/pkg/api/account"
	paymentpb "grpc-bank/pkg/api/payment"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakePaymentRepository struct {
	getFn    func(context.Context, string) (*domain.Transaction, error)
	createFn func(context.Context, *domain.Transaction) error
	updateFn func(context.Context, string, domain.TransactionStatus, string) error
}

func (f *fakePaymentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	if f.getFn != nil {
		return f.getFn(ctx, key)
	}
	return nil, domain.ErrTransactionNotFound
}

func (f *fakePaymentRepository) CreatePending(ctx context.Context, tx *domain.Transaction) error {
	if f.createFn != nil {
		return f.createFn(ctx, tx)
	}
	return nil
}

func (f *fakePaymentRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, errorMsg string) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, id, status, errorMsg)
	}
	return nil
}

type fakeAccountClient struct {
	transferResp *accountpb.ExecuteTransferResponse
	transferErr  error
}

func (f *fakeAccountClient) GetAccount(context.Context, *accountpb.GetAccountRequest, ...grpc.CallOption) (*accountpb.GetAccountResponse, error) {
	return nil, errors.New("unexpected GetAccount call")
}

func (f *fakeAccountClient) CreateAccount(context.Context, *accountpb.CreateAccountRequest, ...grpc.CallOption) (*accountpb.CreateAccountResponse, error) {
	return nil, errors.New("unexpected CreateAccount call")
}

func (f *fakeAccountClient) ExecuteTransfer(context.Context, *accountpb.ExecuteTransferRequest, ...grpc.CallOption) (*accountpb.ExecuteTransferResponse, error) {
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

func newPaymentHandler(repo *fakePaymentRepository, client *fakeAccountClient) *PaymentHandler {
	return NewPaymentHandler(service.NewPaymentService(repo, client))
}

func TestPaymentHandlerCreatePaymentValidation(t *testing.T) {
	tests := []struct {
		name string
		req  *paymentpb.CreatePaymentRequest
	}{
		{name: "missing idempotency key", req: &paymentpb.CreatePaymentRequest{FromAccountId: "acc-1", ToAccountId: "acc-2", Amount: "10"}},
		{name: "missing sender", req: &paymentpb.CreatePaymentRequest{IdempotencyKey: "key-1", ToAccountId: "acc-2", Amount: "10"}},
		{name: "missing recipient", req: &paymentpb.CreatePaymentRequest{IdempotencyKey: "key-1", FromAccountId: "acc-1", Amount: "10"}},
		{name: "invalid amount", req: &paymentpb.CreatePaymentRequest{IdempotencyKey: "key-1", FromAccountId: "acc-1", ToAccountId: "acc-2", Amount: "abc"}},
		{name: "zero amount", req: &paymentpb.CreatePaymentRequest{IdempotencyKey: "key-1", FromAccountId: "acc-1", ToAccountId: "acc-2", Amount: "0"}},
		{name: "negative amount", req: &paymentpb.CreatePaymentRequest{IdempotencyKey: "key-1", FromAccountId: "acc-1", ToAccountId: "acc-2", Amount: "-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newPaymentHandler(&fakePaymentRepository{}, &fakeAccountClient{})
			_, err := h.CreatePayment(context.Background(), tt.req)

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %v", err)
			}
		})
	}
}

func TestPaymentHandlerCreatePaymentFailure(t *testing.T) {
	req := &paymentpb.CreatePaymentRequest{
		IdempotencyKey: "key-1",
		FromAccountId:  "acc-1",
		ToAccountId:    "acc-2",
		Amount:         "10",
	}

	t.Run("maps service error to FailedPrecondition", func(t *testing.T) {
		h := newPaymentHandler(&fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return nil, errors.New("database is down")
			},
		}, &fakeAccountClient{})

		_, err := h.CreatePayment(context.Background(), req)
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})

	t.Run("maps transfer failure to FailedPrecondition", func(t *testing.T) {
		h := newPaymentHandler(&fakePaymentRepository{
			createFn: func(_ context.Context, tx *domain.Transaction) error {
				tx.ID = "tx-1"
				return nil
			},
		}, &fakeAccountClient{transferErr: status.Error(codes.FailedPrecondition, "not enough money")})

		_, err := h.CreatePayment(context.Background(), req)
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})
}

func TestPaymentHandlerCreatePaymentSuccess(t *testing.T) {
	createdAt := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)

	t.Run("returns created payment", func(t *testing.T) {
		h := newPaymentHandler(&fakePaymentRepository{
			createFn: func(_ context.Context, tx *domain.Transaction) error {
				tx.ID = "payment-1"
				tx.CreatedAt = createdAt
				return nil
			},
		}, &fakeAccountClient{})

		resp, err := h.CreatePayment(context.Background(), &paymentpb.CreatePaymentRequest{
			IdempotencyKey: "key-1",
			FromAccountId:  "acc-1",
			ToAccountId:    "acc-2",
			Amount:         "10.5",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.PaymentId != "payment-1" {
			t.Fatalf("expected payment id %q, got %q", "payment-1", resp.PaymentId)
		}
		if resp.Status != string(domain.StatusSuccess) {
			t.Fatalf("expected status %q, got %q", domain.StatusSuccess, resp.Status)
		}
		if resp.CreatedAt != createdAt.Format(time.RFC3339) {
			t.Fatalf("expected created at %q, got %q", createdAt.Format(time.RFC3339), resp.CreatedAt)
		}
	})

	t.Run("returns existing successful payment", func(t *testing.T) {
		existing := &domain.Transaction{
			ID:             "payment-1",
			IdempotencyKey: "key-1",
			Amount:         decimal.NewFromInt(10),
			Status:         domain.StatusSuccess,
			CreatedAt:      createdAt,
		}
		h := newPaymentHandler(&fakePaymentRepository{
			getFn: func(context.Context, string) (*domain.Transaction, error) {
				return existing, nil
			},
		}, &fakeAccountClient{})

		resp, err := h.CreatePayment(context.Background(), &paymentpb.CreatePaymentRequest{
			IdempotencyKey: "key-1",
			FromAccountId:  "acc-1",
			ToAccountId:    "acc-2",
			Amount:         "10",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.PaymentId != "payment-1" || resp.Status != string(domain.StatusSuccess) {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})
}
