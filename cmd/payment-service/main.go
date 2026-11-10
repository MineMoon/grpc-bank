package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"grpc-bank/internal/payment/config"
	"grpc-bank/internal/payment/handler"
	"grpc-bank/internal/payment/repository"
	"grpc-bank/internal/payment/service"
	accountpb "grpc-bank/pkg/api/account"
	paymentpb "grpc-bank/pkg/api/payment"
	"grpc-bank/pkg/env"
	"grpc-bank/pkg/postgres"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	if err := env.Load(env.File()); err != nil {
		log.Fatalf("env load error: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPool, err := postgres.NewPGXPool(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("db connection error: %v", err)
	}
	defer dbPool.Close()
	log.Println("Success db concection (postgres: payments_db)")

	accountConn, err := grpc.NewClient(
		cfg.AccountServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("account service client connection error: %v", err)
	}
	defer func() {
		if err := accountConn.Close(); err != nil {
			log.Printf("error closing account service connection: %v", err)
		}
	}()

	accountClient := accountpb.NewAccountServiceClient(accountConn)

	paymentRepo := repository.NewPaymentRepository(dbPool)
	paymentService := service.NewPaymentService(paymentRepo, accountClient)
	paymentHandler := handler.NewPaymentHandler(paymentService)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("tcp port open error :%s: %v", cfg.GRPCPort, err)
	}

	grpcServer := grpc.NewServer()
	paymentpb.RegisterPaymentServiceServer(grpcServer, paymentHandler)
	reflection.Register(grpcServer) /// - ОПАСНО, СДЕЛАНО ДЛЯ ТЕСТОВ НА ВРЕМЯ, УБРАТЬ

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Termination signal received. Stopping gRPC server...")
		grpcServer.GracefulStop()
		cancel()
	}()

	log.Printf("Payment Service has been successfully started on the gRPC port :%s\n", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("gRPC server crashed: %v", err)
	}

	log.Println("Payment Service has been stopped correctly.")
}
