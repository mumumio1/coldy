package grpc

import (
	"context"

	commonv1 "github.com/mumumio1/coldy/proto/common/v1"
	usersv1 "github.com/mumumio1/coldy/proto/users/v1"
	"github.com/mumumio1/coldy/services/users/internal/service"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the User gRPC service
type Server struct {
	usersv1.UnimplementedUserServiceServer
	userService *service.UserService
	logger      *zap.Logger
}

// NewServer creates a new gRPC server
func NewServer(userService *service.UserService, logger *zap.Logger) *Server {
	return &Server{
		userService: userService,
		logger:      logger,
	}
}

// Register registers a new user
func (s *Server) Register(ctx context.Context, req *usersv1.RegisterRequest) (*usersv1.RegisterResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	user, accessToken, refreshToken, err := s.userService.Register(
		ctx,
		req.Email,
		req.Password,
		req.FullName,
		req.Phone,
	)
	if err != nil {
		s.logger.Error("failed to register user", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to register user")
	}

	return &usersv1.RegisterResponse{
		User: &usersv1.User{
			Id:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Phone:     user.Phone,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Login authenticates a user
func (s *Server) Login(ctx context.Context, req *usersv1.LoginRequest) (*usersv1.LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	user, accessToken, refreshToken, err := s.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		s.logger.Error("failed to login", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	return &usersv1.LoginResponse{
		User: &usersv1.User{
			Id:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Phone:     user.Phone,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// GetUser retrieves a user by ID
func (s *Server) GetUser(ctx context.Context, req *usersv1.GetUserRequest) (*usersv1.GetUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	user, err := s.userService.GetUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error("failed to get user", zap.Error(err))
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &usersv1.GetUserResponse{
		User: &usersv1.User{
			Id:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Phone:     user.Phone,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		},
	}, nil
}

// UpdateUser updates a user
func (s *Server) UpdateUser(ctx context.Context, req *usersv1.UpdateUserRequest) (*usersv1.UpdateUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	user, err := s.userService.UpdateUser(ctx, req.UserId, req.FullName, req.Phone)
	if err != nil {
		s.logger.Error("failed to update user", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update user")
	}

	return &usersv1.UpdateUserResponse{
		User: &usersv1.User{
			Id:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Phone:     user.Phone,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		},
	}, nil
}

// ListUsers lists users with pagination
func (s *Server) ListUsers(ctx context.Context, req *usersv1.ListUsersRequest) (*usersv1.ListUsersResponse, error) {
	pageSize := int(req.Pagination.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	users, nextCursor, hasMore, err := s.userService.ListUsers(ctx, pageSize, req.Pagination.Cursor)
	if err != nil {
		s.logger.Error("failed to list users", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list users")
	}

	protoUsers := make([]*usersv1.User, len(users))
	for i, user := range users {
		protoUsers[i] = &usersv1.User{
			Id:        user.ID,
			Email:     user.Email,
			FullName:  user.FullName,
			Phone:     user.Phone,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		}
	}

	return &usersv1.ListUsersResponse{
		Users: protoUsers,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	}, nil
}
