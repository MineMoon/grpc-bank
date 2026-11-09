package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accountpb "grpc-bank/pkg/api/account"
	paymentpb "grpc-bank/pkg/api/payment"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAccountClient struct {
	createResp *accountpb.CreateAccountResponse
	createErr  error

	getResp *accountpb.GetAccountResponse
	getErr  error

	depositResp *accountpb.DepositResponse
	depositErr  error

	transferResp *accountpb.ExecuteTransferResponse
	transferErr  error
}

func (f *fakeAccountClient) CreateAccount(context.Context, *accountpb.CreateAccountRequest, ...grpc.CallOption) (*accountpb.CreateAccountResponse, error) {
	return f.createResp, f.createErr
}

func (f *fakeAccountClient) GetAccount(context.Context, *accountpb.GetAccountRequest, ...grpc.CallOption) (*accountpb.GetAccountResponse, error) {
	return f.getResp, f.getErr
}

func (f *fakeAccountClient) ExecuteTransfer(context.Context, *accountpb.ExecuteTransferRequest, ...grpc.CallOption) (*accountpb.ExecuteTransferResponse, error) {
	return f.transferResp, f.transferErr
}

func (f *fakeAccountClient) Deposit(context.Context, *accountpb.DepositRequest, ...grpc.CallOption) (*accountpb.DepositResponse, error) {
	return f.depositResp, f.depositErr
}

type fakePaymentClient struct {
	createResp *paymentpb.CreatePaymentResponse
	createErr  error
}

func (f *fakePaymentClient) CreatePayment(context.Context, *paymentpb.CreatePaymentRequest, ...grpc.CallOption) (*paymentpb.CreatePaymentResponse, error) {
	return f.createResp, f.createErr
}

func newGatewayHandler() (*GatewayHandler, *fakeAccountClient, *fakePaymentClient) {
	accountClient := &fakeAccountClient{}
	paymentClient := &fakePaymentClient{}
	return NewGatewayHandler(accountClient, paymentClient), accountClient, paymentClient
}

func doRequest(serve func(http.ResponseWriter, *http.Request), method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	serve(rec, req)
	return rec
}

func TestGatewayCreateAccount(t *testing.T) {
	t.Run("creates account", func(t *testing.T) {
		h, accountClient, _ := newGatewayHandler()
		accountClient.createResp = &accountpb.CreateAccountResponse{
			AccountId:     "acc-1",
			AccountNumber: "40817810000000000001",
		}

		rec := doRequest(h.CreateAccount, http.MethodPost, "/api/v1/accounts", `{"user_id":"user-1","currency":"RUB"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
		}

		var resp CreateAccountResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.AccountId != "acc-1" || resp.AccountNumber != "40817810000000000001" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("rejects invalid body", func(t *testing.T) {
		h, _, _ := newGatewayHandler()
		rec := doRequest(h.CreateAccount, http.MethodPost, "/api/v1/accounts", `{`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("maps grpc errors", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			wantCode int
		}{
			{name: "not found", err: status.Error(codes.NotFound, "missing"), wantCode: http.StatusNotFound},
			{name: "already exists", err: status.Error(codes.AlreadyExists, "duplicate"), wantCode: http.StatusConflict},
			{name: "invalid argument", err: status.Error(codes.InvalidArgument, "bad"), wantCode: http.StatusBadRequest},
			{name: "failed precondition", err: status.Error(codes.FailedPrecondition, "rejected"), wantCode: http.StatusUnprocessableEntity},
			{name: "internal", err: status.Error(codes.Internal, "boom"), wantCode: http.StatusInternalServerError},
			{name: "non grpc error", err: errors.New("boom"), wantCode: http.StatusInternalServerError},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h, accountClient, _ := newGatewayHandler()
				accountClient.createErr = tt.err

				rec := doRequest(h.CreateAccount, http.MethodPost, "/api/v1/accounts", `{"user_id":"user-1"}`)
				if rec.Code != tt.wantCode {
					t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
				}
			})
		}
	})
}

func TestGatewayGetAccount(t *testing.T) {
	routerWithAccount := func(h *GatewayHandler) http.Handler {
		r := chi.NewRouter()
		r.Get("/api/v1/accounts/{id}", h.GetAccount)
		return r
	}

	t.Run("returns account", func(t *testing.T) {
		h, accountClient, _ := newGatewayHandler()
		accountClient.getResp = &accountpb.GetAccountResponse{
			AccountId:     "acc-1",
			UserId:        "user-1",
			AccountNumber: "40817810000000000001",
			Balance:       "123.45",
			Currency:      "RUB",
		}

		rec := doRequest(routerWithAccount(h).ServeHTTP, http.MethodGet, "/api/v1/accounts/acc-1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var resp AccountResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.AccountId != "acc-1" || resp.Balance != "123.45" || resp.Currency != "RUB" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("rejects missing account id", func(t *testing.T) {
		h, _, _ := newGatewayHandler()
		rec := doRequest(h.GetAccount, http.MethodGet, "/api/v1/accounts", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("maps missing account to not found", func(t *testing.T) {
		h, accountClient, _ := newGatewayHandler()
		accountClient.getErr = status.Error(codes.NotFound, "account not found")

		rec := doRequest(routerWithAccount(h).ServeHTTP, http.MethodGet, "/api/v1/accounts/acc-1", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})
}

func TestGatewayDeposit(t *testing.T) {
	t.Run("returns new balance", func(t *testing.T) {
		h, accountClient, _ := newGatewayHandler()
		accountClient.depositResp = &accountpb.DepositResponse{Status: "SUCCESS", BalanceAfter: "110"}

		rec := doRequest(h.Deposit, http.MethodPost, "/api/v1/accounts/deposit", `{"account_id":"acc-1","amount":"10"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var resp DepositResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Status != "SUCCESS" || resp.BalanceAfter != "110" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("rejects invalid body", func(t *testing.T) {
		h, _, _ := newGatewayHandler()
		rec := doRequest(h.Deposit, http.MethodPost, "/api/v1/accounts/deposit", `{`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("maps grpc failure", func(t *testing.T) {
		h, accountClient, _ := newGatewayHandler()
		accountClient.depositErr = status.Error(codes.FailedPrecondition, "rejected")

		rec := doRequest(h.Deposit, http.MethodPost, "/api/v1/accounts/deposit", `{"account_id":"acc-1","amount":"10"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, rec.Code)
		}
	})
}

func TestGatewayCreateTransfer(t *testing.T) {
	t.Run("returns payment", func(t *testing.T) {
		h, _, paymentClient := newGatewayHandler()
		paymentClient.createResp = &paymentpb.CreatePaymentResponse{
			PaymentId: "payment-1",
			Status:    "SUCCESS",
			CreatedAt: "2026-11-09T12:00:00Z",
		}

		rec := doRequest(h.CreateTransfer, http.MethodPost, "/api/v1/transfers", `{
			"idempotency_key":"key-1",
			"from_account_id":"acc-1",
			"to_account_id":"acc-2",
			"amount":"10.5"
		}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var resp paymentpb.CreatePaymentResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.PaymentId != "payment-1" || resp.Status != "SUCCESS" {
			t.Fatalf("unexpected response: payment_id=%q status=%q", resp.PaymentId, resp.Status)
		}
	})

	t.Run("maps grpc failure", func(t *testing.T) {
		h, _, paymentClient := newGatewayHandler()
		paymentClient.createErr = status.Error(codes.FailedPrecondition, "rejected")

		rec := doRequest(h.CreateTransfer, http.MethodPost, "/api/v1/transfers", `{"idempotency_key":"key-1"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, rec.Code)
		}
	})
}

func TestHandleGRPCErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "not found", err: status.Error(codes.NotFound, "missing"), wantCode: http.StatusNotFound},
		{name: "already exists", err: status.Error(codes.AlreadyExists, "duplicate"), wantCode: http.StatusConflict},
		{name: "invalid argument", err: status.Error(codes.InvalidArgument, "bad"), wantCode: http.StatusBadRequest},
		{name: "failed precondition", err: status.Error(codes.FailedPrecondition, "rejected"), wantCode: http.StatusUnprocessableEntity},
		{name: "other grpc code", err: status.Error(codes.PermissionDenied, "denied"), wantCode: http.StatusInternalServerError},
		{name: "non grpc error", err: errors.New("plain error"), wantCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handleGRPCErr(rec, tt.err)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d", tt.wantCode, rec.Code)
			}
			if rec.Body.Len() == 0 {
				t.Fatal("expected error body")
			}
		})
	}
}
