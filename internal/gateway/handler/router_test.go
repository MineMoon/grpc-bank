package handler

import (
	"net/http"
	"testing"

	accountpb "grpc-bank/pkg/api/account"
	paymentpb "grpc-bank/pkg/api/payment"
)

func TestNewRouter(t *testing.T) {
	h, accountClient, paymentClient := newGatewayHandler()
	accountClient.createResp = &accountpb.CreateAccountResponse{AccountId: "acc-1", AccountNumber: "40817810000000000001"}
	accountClient.getResp = &accountpb.GetAccountResponse{AccountId: "acc-1"}
	accountClient.depositResp = &accountpb.DepositResponse{Status: "SUCCESS", BalanceAfter: "10"}
	paymentClient.createResp = &paymentpb.CreatePaymentResponse{PaymentId: "payment-1", Status: "SUCCESS"}

	router := NewRouter(h)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{name: "create account", method: http.MethodPost, path: "/api/v1/accounts", body: `{"user_id":"user-1"}`, wantCode: http.StatusCreated},
		{name: "get account", method: http.MethodGet, path: "/api/v1/accounts/acc-1", wantCode: http.StatusOK},
		{name: "deposit", method: http.MethodPost, path: "/api/v1/accounts/deposit", body: `{"account_id":"acc-1","amount":"10"}`, wantCode: http.StatusOK},
		{name: "create transfer", method: http.MethodPost, path: "/api/v1/transfers", body: `{"idempotency_key":"key-1","from_account_id":"acc-1","to_account_id":"acc-2","amount":"10"}`, wantCode: http.StatusOK},
		{name: "unknown route", method: http.MethodGet, path: "/api/v1/unknown", wantCode: http.StatusNotFound},
		{name: "wrong method", method: http.MethodGet, path: "/api/v1/accounts", wantCode: http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(router.ServeHTTP, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}
