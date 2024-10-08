package grpc

import (
	"context"

	catalogv1 "github.com/mumumio1/coldy/proto/catalog/v1"
	commonv1 "github.com/mumumio1/coldy/proto/common/v1"
	"github.com/mumumio1/coldy/services/catalog/internal/repository"
	"github.com/mumumio1/coldy/services/catalog/internal/service"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the Catalog gRPC service
type Server struct {
	catalogv1.UnimplementedCatalogServiceServer
	catalogService *service.CatalogService
	logger         *zap.Logger
}

// NewServer creates a new gRPC server
func NewServer(catalogService *service.CatalogService, logger *zap.Logger) *Server {
	return &Server{
		catalogService: catalogService,
		logger:         logger,
	}
}

// GetProduct retrieves a product
func (s *Server) GetProduct(ctx context.Context, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	if req.ProductId == "" {
		return nil, status.Error(codes.InvalidArgument, "product_id is required")
	}

	product, err := s.catalogService.GetProduct(ctx, req.ProductId)
	if err != nil {
		s.logger.Error("failed to get product", zap.Error(err))
		return nil, status.Error(codes.NotFound, "product not found")
	}

	return &catalogv1.GetProductResponse{
		Product: toProtoProduct(product),
	}, nil
}

// ListProducts lists products
func (s *Server) ListProducts(ctx context.Context, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	pageSize := int(req.Pagination.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	products, nextCursor, hasMore, err := s.catalogService.ListProducts(
		ctx,
		pageSize,
		req.Pagination.Cursor,
		req.Category,
		req.SearchQuery,
	)
	if err != nil {
		s.logger.Error("failed to list products", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list products")
	}

	protoProducts := make([]*catalogv1.Product, len(products))
	for i, product := range products {
		protoProducts[i] = toProtoProduct(product)
	}

	return &catalogv1.ListProductsResponse{
		Products: protoProducts,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: nextCursor,
			HasMore:    hasMore,
		},
	}, nil
}

// CreateProduct creates a new product
func (s *Server) CreateProduct(ctx context.Context, req *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error) {
	if req.Name == "" || req.Sku == "" {
		return nil, status.Error(codes.InvalidArgument, "name and sku are required")
	}

	product := &repository.Product{
		Name:          req.Name,
		Description:   req.Description,
		SKU:           req.Sku,
		PriceCurrency: req.Price.Currency,
		PriceAmount:   req.Price.Amount,
		StockQuantity: req.StockQuantity,
		Category:      req.Category,
		ImageURLs:     req.ImageUrls,
	}

	if err := s.catalogService.CreateProduct(ctx, product); err != nil {
		s.logger.Error("failed to create product", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create product")
	}

	return &catalogv1.CreateProductResponse{
		Product: toProtoProduct(product),
	}, nil
}

// UpdateProduct updates a product
func (s *Server) UpdateProduct(ctx context.Context, req *catalogv1.UpdateProductRequest) (*catalogv1.UpdateProductResponse, error) {
	if req.ProductId == "" {
		return nil, status.Error(codes.InvalidArgument, "product_id is required")
	}

	// Get existing product
	product, err := s.catalogService.GetProduct(ctx, req.ProductId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "product not found")
	}

	// Update fields
	if req.Name != "" {
		product.Name = req.Name
	}
	if req.Description != "" {
		product.Description = req.Description
	}
	if req.Price != nil {
		product.PriceCurrency = req.Price.Currency
		product.PriceAmount = req.Price.Amount
	}
	if req.Category != "" {
		product.Category = req.Category
	}

	if err := s.catalogService.UpdateProduct(ctx, product); err != nil {
		s.logger.Error("failed to update product", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update product")
	}

	return &catalogv1.UpdateProductResponse{
		Product: toProtoProduct(product),
	}, nil
}

// UpdateStock updates product stock
func (s *Server) UpdateStock(ctx context.Context, req *catalogv1.UpdateStockRequest) (*catalogv1.UpdateStockResponse, error) {
	if req.ProductId == "" {
		return nil, status.Error(codes.InvalidArgument, "product_id is required")
	}

	newQuantity, err := s.catalogService.UpdateStock(ctx, req.ProductId, req.QuantityDelta)
	if err != nil {
		s.logger.Error("failed to update stock", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update stock")
	}

	return &catalogv1.UpdateStockResponse{
		NewStockQuantity: newQuantity,
	}, nil
}

// CheckAvailability checks product availability
func (s *Server) CheckAvailability(ctx context.Context, req *catalogv1.CheckAvailabilityRequest) (*catalogv1.CheckAvailabilityResponse, error) {
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "items are required")
	}

	items := make(map[string]int32)
	for _, item := range req.Items {
		items[item.ProductId] = item.Quantity
	}

	unavailable, err := s.catalogService.CheckAvailability(ctx, items)
	if err != nil {
		s.logger.Error("failed to check availability", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to check availability")
	}

	protoUnavailable := make([]*catalogv1.UnavailableItem, len(unavailable))
	for i, item := range unavailable {
		protoUnavailable[i] = &catalogv1.UnavailableItem{
			ProductId: item.ProductID,
			Requested: item.Requested,
			Available: item.Available,
		}
	}

	return &catalogv1.CheckAvailabilityResponse{
		Available:        len(unavailable) == 0,
		UnavailableItems: protoUnavailable,
	}, nil
}

func toProtoProduct(product *repository.Product) *catalogv1.Product {
	return &catalogv1.Product{
		Id:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		Sku:         product.SKU,
		Price: &commonv1.Money{
			Currency: product.PriceCurrency,
			Amount:   product.PriceAmount,
		},
		StockQuantity: product.StockQuantity,
		Category:      product.Category,
		ImageUrls:     product.ImageURLs,
		CreatedAt:     timestamppb.New(product.CreatedAt),
		UpdatedAt:     timestamppb.New(product.UpdatedAt),
	}
}
