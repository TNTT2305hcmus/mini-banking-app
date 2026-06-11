package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mini-banking/pkg/pb/bank"
)

type Handler struct {
	pb.UnimplementedBankServiceServer
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) CreateUser(_ context.Context, _ *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method CreateUser not implemented")
}

func (h *Handler) TransferMoney(_ context.Context, _ *pb.TransferRequest) (*pb.TransferResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method TransferMoney not implemented")
}

func (h *Handler) GetBalance(_ context.Context, _ *pb.BalanceRequest) (*pb.BalanceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method GetBalance not implemented")
}

func (h *Handler) GetHistory(_ context.Context, _ *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method GetHistory not implemented")
}
