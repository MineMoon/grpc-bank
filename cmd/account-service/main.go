package main

import (
	"context"
	"fmt"
	"grpc-bank/internal/account/config"
	"grpc-bank/internal/account/handler"
	"grpc-bank/internal/account/repository"
	"grpc-bank/internal/account/service"
	accountpb "grpc-bank/pkg/api/account"
	"grpc-bank/pkg/env"
	"grpc-bank/pkg/postgres"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
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
	log.Println("Success db concection (postgres: account_db)")

	accountRepo := repository.NewAccountReposiroty(dbPool)
	accountService := service.NewAccountService(accountRepo)
	accountHandler := handler.NewAccountHandler(accountService)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("tcp port open error :%s: %v", cfg.GRPCPort, err)
	}

	grpcServer := grpc.NewServer()
	accountpb.RegisterAccountServiceServer(grpcServer, accountHandler)
	reflection.Register(grpcServer) /// - ОПАСНО, СДЕЛАНО ДЛЯ ТЕСТОВ НА ВРЕМЯ, УБРАТЬ

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Termination signal received. Stopping gRPC server...")
		grpcServer.GracefulStop()
		cancel()
	}()

	log.Printf("Account Service has been successfully started on the gRPC port :%s\n", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("gRPC server crashed: %v", err)
	}

	log.Println("Account Service has been stopped correctly.")

}
