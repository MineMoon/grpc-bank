package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grpc-bank/internal/gateway/config"
	"grpc-bank/internal/gateway/handler"
	accountpb "grpc-bank/pkg/api/account"
	paymentpb "grpc-bank/pkg/api/payment"
	"grpc-bank/pkg/env"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := env.Load(env.File()); err != nil {
		log.Fatalf("env load error: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}

	accConn, err := grpc.NewClient(cfg.AccountServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to account service: %v", err)
	}
	defer func() { _ = accConn.Close() }()
	accClient := accountpb.NewAccountServiceClient(accConn)

	payConn, err := grpc.NewClient(cfg.PaymentServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to payment service: %v", err)
	}
	defer func() { _ = payConn.Close() }()
	payClient := paymentpb.NewPaymentServiceClient(payConn)

	// Собираем хэндлер и передаем его в роутер
	h := handler.NewGatewayHandler(accClient, payClient)
	r := handler.NewRouter(h)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler: r,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down API Gateway...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("API Gateway successfully started on HTTP port :%s\n", cfg.HTTPPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server crashed: %v", err)
	}
	log.Println("API Gateway stopped correctly.")
}
