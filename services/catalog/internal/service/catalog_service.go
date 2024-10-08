package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mumumio1/coldy/pkg/cache"
	"github.com/mumumio1/coldy/services/catalog/internal/repository"
	"go.uber.org/zap"
)

const (
	// Cache TTLs
	ProductCacheTTL = 5 * time.Minute
	ListCacheTTL    = 2 * time.Minute

	// Cache key prefixes
	ProductCachePrefix = "product:"
	ListCachePrefix    = "products:list:"
)

// CatalogService handles catalog business logic
type CatalogService struct {
	repo   *repository.ProductRepository
	cache  *cache.RedisCache
	logger *zap.Logger
}

// NewCatalogService creates a new catalog service
func NewCatalogService(repo *repository.ProductRepository, cache *cache.RedisCache, logger *zap.Logger) *CatalogService {
	return &CatalogService{
		repo:   repo,
		cache:  cache,
		logger: logger,
	}
}

// GetProduct retrieves a product with cache
func (s *CatalogService) GetProduct(ctx context.Context, productID string) (*repository.Product, error) {
	cacheKey := ProductCachePrefix + productID

	// Try cache first (read-through pattern)
	var product repository.Product
	found, err := s.cache.GetJSON(ctx, cacheKey, &product)
	if err != nil {
		s.logger.Warn("cache get failed", zap.Error(err))
	}
	if found {
		s.logger.Debug("cache hit", zap.String("product_id", productID))
		return &product, nil
	}

	// Cache miss - fetch from database
	s.logger.Debug("cache miss", zap.String("product_id", productID))
	productPtr, err := s.repo.GetByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}
	if productPtr == nil {
		return nil, fmt.Errorf("product not found")
	}

	// Store in cache
	if err := s.cache.SetJSON(ctx, cacheKey, productPtr, ProductCacheTTL); err != nil {
		s.logger.Warn("cache set failed", zap.Error(err))
	}

	return productPtr, nil
}

// CreateProduct creates a new product
func (s *CatalogService) CreateProduct(ctx context.Context, product *repository.Product) error {
	if err := s.repo.Create(ctx, product); err != nil {
		return fmt.Errorf("failed to create product: %w", err)
	}

	// Invalidate list cache
	s.invalidateListCache(ctx)

	s.logger.Info("product created", zap.String("product_id", product.ID))
	return nil
}

// UpdateProduct updates a product
func (s *CatalogService) UpdateProduct(ctx context.Context, product *repository.Product) error {
	if err := s.repo.Update(ctx, product); err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	// Invalidate cache
	cacheKey := ProductCachePrefix + product.ID
	if err := s.cache.Delete(ctx, cacheKey); err != nil {
		s.logger.Warn("cache delete failed", zap.Error(err))
	}

	// Invalidate list cache
	s.invalidateListCache(ctx)

	s.logger.Info("product updated", zap.String("product_id", product.ID))
	return nil
}

// UpdateStock updates product stock
func (s *CatalogService) UpdateStock(ctx context.Context, productID string, delta int32) (int32, error) {
	newQuantity, err := s.repo.UpdateStock(ctx, productID, delta)
	if err != nil {
		return 0, fmt.Errorf("failed to update stock: %w", err)
	}

	// Invalidate cache
	cacheKey := ProductCachePrefix + productID
	if err := s.cache.Delete(ctx, cacheKey); err != nil {
		s.logger.Warn("cache delete failed", zap.Error(err))
	}

	s.logger.Info("stock updated",
		zap.String("product_id", productID),
		zap.Int32("delta", delta),
		zap.Int32("new_quantity", newQuantity),
	)

	return newQuantity, nil
}

// ListProducts lists products with caching
func (s *CatalogService) ListProducts(ctx context.Context, limit int, cursor, category, searchQuery string) ([]*repository.Product, string, bool, error) {
	// Generate cache key
	cacheKey := s.generateListCacheKey(limit, cursor, category, searchQuery)

	// Try cache first
	type cachedList struct {
		Products   []*repository.Product `json:"products"`
		NextCursor string                `json:"next_cursor"`
	}

	var cached cachedList
	found, err := s.cache.GetJSON(ctx, cacheKey, &cached)
	if err != nil {
		s.logger.Warn("cache get failed", zap.Error(err))
	}
	if found {
		s.logger.Debug("list cache hit")
		return cached.Products, cached.NextCursor, cached.NextCursor != "", nil
	}

	// Cache miss - fetch from database
	s.logger.Debug("list cache miss")
	products, nextCursor, err := s.repo.List(ctx, limit, cursor, category, searchQuery)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to list products: %w", err)
	}

	// Store in cache
	cached = cachedList{
		Products:   products,
		NextCursor: nextCursor,
	}
	if err := s.cache.SetJSON(ctx, cacheKey, cached, ListCacheTTL); err != nil {
		s.logger.Warn("cache set failed", zap.Error(err))
	}

	hasMore := nextCursor != ""
	return products, nextCursor, hasMore, nil
}

// CheckAvailability checks if products have sufficient stock
func (s *CatalogService) CheckAvailability(ctx context.Context, items map[string]int32) ([]UnavailableItem, error) {
	available, err := s.repo.CheckAvailability(ctx, items)
	if err != nil {
		return nil, fmt.Errorf("failed to check availability: %w", err)
	}

	var unavailable []UnavailableItem
	for productID, requestedQty := range items {
		availableQty, exists := available[productID]
		if !exists {
			unavailable = append(unavailable, UnavailableItem{
				ProductID: productID,
				Requested: requestedQty,
				Available: 0,
			})
			continue
		}

		if availableQty < requestedQty {
			unavailable = append(unavailable, UnavailableItem{
				ProductID: productID,
				Requested: requestedQty,
				Available: availableQty,
			})
		}
	}

	return unavailable, nil
}

// UnavailableItem represents an unavailable product
type UnavailableItem struct {
	ProductID string
	Requested int32
	Available int32
}

func (s *CatalogService) generateListCacheKey(limit int, cursor, category, searchQuery string) string {
	data := map[string]interface{}{
		"limit":  limit,
		"cursor": cursor,
		"cat":    category,
		"search": searchQuery,
	}
	jsonData, _ := json.Marshal(data)
	return ListCachePrefix + string(jsonData)
}

func (s *CatalogService) invalidateListCache(_ context.Context) {
	// In production, use Redis SCAN to find and delete all list cache keys
	s.logger.Debug("invalidating list cache")
}
