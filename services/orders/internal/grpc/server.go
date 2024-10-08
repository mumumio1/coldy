package grpc

import (
	"context"

	commonv1 "github.com/mumumio1/coldy/proto/common/v1"
	ordersv1 "github.com/mumumio1/coldy/proto/orders/v1"
	"github.com/mumumio1/coldy/services/orders/internal/repository"
	"github.com/mumumio1/coldy/services/orders/internal/service"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the Order gRPC service
type Server struct {
	ordersv1.UnimplementedOrderServiceServer
	orderService *service.OrderService
	logger       *zap.Logger
}

// NewServer creates a new gRPC server
func NewServer(orderService *service.OrderService, logger *zap.Logger) *Server {
	return &Server{
		orderService: orderService,
		logger:       logger,
	}
}

// CreateOrder creates a new order
func (s *Server) CreateOrder(ctx context.Context, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "items are required")
	}

	// Convert request items
	items := make([]service.OrderItemRequest, len(req.Items))
	for i, item := range req.Items {
		items[i] = service.OrderItemRequest{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		}
	}

	// Create order request
	orderReq := &service.CreateOrderRequest{
		UserID:             req.UserId,
		Items:              items,
		ShippingStreet:     req.ShippingAddress.Street,
		ShippingCity:       req.ShippingAddress.City,
		ShippingState:      req.ShippingAddress.State,
		ShippingPostalCode: req.ShippingAddress.PostalCode,
		ShippingCountry:    req.ShippingAddress.Country,
	}

	order, fromCache, err := s.orderService.CreateOrder(ctx, req.IdempotencyKey, orderReq)
	if err != nil {
		s.logger.Error("failed to create order", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create order")
	}

	return &ordersv1.CreateOrderResponse{
		Order:     toProtoOrder(order),
		FromCache: fromCache,
	}, nil
}

// GetOrder retrieves an order
func (s *Server) GetOrder(ctx context.Context, req *ordersv1.GetOrderRequest) (*ordersv1.GetOrderResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := s.orderService.GetOrder(ctx, req.OrderId)
	if err != nil {
		s.logger.Error("failed to get order", zap.Error(err))
		return nil, status.Error(codes.NotFound, "order not found")
	}

	return &ordersv1.GetOrderResponse{
		Order: toProtoOrder(order),
	}, nil
}

// ListOrders lists orders
func (s *Server) ListOrders(ctx context.Context, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	pageSize := int(req.Pagination.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	orderStatus := repository.OrderStatus("")
	if req.StatusFilter != ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		orderStatus = toRepoStatus(req.StatusFilter)
	}

	orders, nextCursor, hasMore, err := s.orderService.ListOrders(
		ctx,
		req.UserId,
		orderStatus,
		pageSize,
		req.Pagination.Cursor,
	)
	if err != nil {
		s.logger.Error("failed to list orders", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list orders")
	}

	protoOrders := make([]*ordersv1.Order, len(orders))
	for i, order := range orders {
		protoOrders[i] = toProtoOrder(order)
	}

	return &ordersv1.ListOrdersResponse{
		Orders: protoOrders,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	}, nil
}

// CancelOrder cancels an order
func (s *Server) CancelOrder(ctx context.Context, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	if err := s.orderService.CancelOrder(ctx, req.OrderId, req.Reason); err != nil {
		s.logger.Error("failed to cancel order", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to cancel order")
	}

	order, err := s.orderService.GetOrder(ctx, req.OrderId)
	if err != nil {
		s.logger.Error("failed to get order", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get order")
	}

	return &ordersv1.CancelOrderResponse{
		Order: toProtoOrder(order),
	}, nil
}

// UpdateOrderStatus updates order status
func (s *Server) UpdateOrderStatus(ctx context.Context, req *ordersv1.UpdateOrderStatusRequest) (*ordersv1.UpdateOrderStatusResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}

	repoStatus := toRepoStatus(req.Status)
	if err := s.orderService.UpdateOrderStatus(ctx, req.OrderId, repoStatus); err != nil {
		s.logger.Error("failed to update order status", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update order status")
	}

	order, err := s.orderService.GetOrder(ctx, req.OrderId)
	if err != nil {
		s.logger.Error("failed to get order", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get order")
	}

	return &ordersv1.UpdateOrderStatusResponse{
		Order: toProtoOrder(order),
	}, nil
}

func toProtoOrder(order *repository.Order) *ordersv1.Order {
	items := make([]*ordersv1.OrderItem, len(order.Items))
	for i, item := range order.Items {
		items[i] = &ordersv1.OrderItem{
			ProductId:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice: &commonv1.Money{
				Currency: item.UnitPriceCurrency,
				Amount:   item.UnitPriceAmount,
			},
			TotalPrice: &commonv1.Money{
				Currency: item.TotalPriceCurrency,
				Amount:   item.TotalPriceAmount,
			},
		}
	}

	return &ordersv1.Order{
		Id:     order.ID,
		UserId: order.UserID,
		Items:  items,
		TotalAmount: &commonv1.Money{
			Currency: order.TotalCurrency,
			Amount:   order.TotalAmount,
		},
		Status:    toProtoStatus(order.Status),
		PaymentId: order.PaymentID,
		ShippingAddress: &commonv1.Address{
			Street:     order.ShippingStreet,
			City:       order.ShippingCity,
			State:      order.ShippingState,
			PostalCode: order.ShippingPostalCode,
			Country:    order.ShippingCountry,
		},
		CreatedAt: timestamppb.New(order.CreatedAt),
		UpdatedAt: timestamppb.New(order.UpdatedAt),
	}
}

func toProtoStatus(status repository.OrderStatus) ordersv1.OrderStatus {
	switch status {
	case repository.StatusPending:
		return ordersv1.OrderStatus_ORDER_STATUS_PENDING
	case repository.StatusConfirmed:
		return ordersv1.OrderStatus_ORDER_STATUS_CONFIRMED
	case repository.StatusPaid:
		return ordersv1.OrderStatus_ORDER_STATUS_PAID
	case repository.StatusProcessing:
		return ordersv1.OrderStatus_ORDER_STATUS_PROCESSING
	case repository.StatusShipped:
		return ordersv1.OrderStatus_ORDER_STATUS_SHIPPED
	case repository.StatusDelivered:
		return ordersv1.OrderStatus_ORDER_STATUS_DELIVERED
	case repository.StatusCancelled:
		return ordersv1.OrderStatus_ORDER_STATUS_CANCELED
	case repository.StatusRefunded:
		return ordersv1.OrderStatus_ORDER_STATUS_REFUNDED
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func toRepoStatus(status ordersv1.OrderStatus) repository.OrderStatus {
	switch status {
	case ordersv1.OrderStatus_ORDER_STATUS_PENDING:
		return repository.StatusPending
	case ordersv1.OrderStatus_ORDER_STATUS_CONFIRMED:
		return repository.StatusConfirmed
	case ordersv1.OrderStatus_ORDER_STATUS_PAID:
		return repository.StatusPaid
	case ordersv1.OrderStatus_ORDER_STATUS_PROCESSING:
		return repository.StatusProcessing
	case ordersv1.OrderStatus_ORDER_STATUS_SHIPPED:
		return repository.StatusShipped
	case ordersv1.OrderStatus_ORDER_STATUS_DELIVERED:
		return repository.StatusDelivered
	case ordersv1.OrderStatus_ORDER_STATUS_CANCELED:
		return repository.StatusCancelled
	case ordersv1.OrderStatus_ORDER_STATUS_REFUNDED:
		return repository.StatusRefunded
	default:
		return repository.StatusPending
	}
}
