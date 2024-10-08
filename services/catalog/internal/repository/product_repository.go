package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Product represents a product entity
type Product struct {
	ID            string
	Name          string
	Description   string
	SKU           string
	PriceCurrency string
	PriceAmount   int64
	StockQuantity int32
	Category      string
	ImageURLs     []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProductRepository handles product data access
type ProductRepository struct {
	db *sql.DB
}

// NewProductRepository creates a new product repository
func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// Create creates a new product
func (r *ProductRepository) Create(ctx context.Context, product *Product) error {
	query := `
		INSERT INTO products (id, name, description, sku, price_currency, price_amount, stock_quantity, category, image_urls)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at
	`

	product.ID = uuid.New().String()

	err := r.db.QueryRowContext(ctx, query,
		product.ID,
		product.Name,
		product.Description,
		product.SKU,
		product.PriceCurrency,
		product.PriceAmount,
		product.StockQuantity,
		product.Category,
		pq.Array(product.ImageURLs),
	).Scan(&product.CreatedAt, &product.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create product: %w", err)
	}

	return nil
}

// GetByID retrieves a product by ID
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*Product, error) {
	query := `
		SELECT id, name, description, sku, price_currency, price_amount, stock_quantity, category, image_urls, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var product Product
	var imageURLs pq.StringArray

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&product.ID,
		&product.Name,
		&product.Description,
		&product.SKU,
		&product.PriceCurrency,
		&product.PriceAmount,
		&product.StockQuantity,
		&product.Category,
		&imageURLs,
		&product.CreatedAt,
		&product.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	product.ImageURLs = imageURLs
	return &product, nil
}

// Update updates a product
func (r *ProductRepository) Update(ctx context.Context, product *Product) error {
	query := `
		UPDATE products
		SET name = $1, description = $2, price_currency = $3, price_amount = $4, category = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $6
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		product.Name,
		product.Description,
		product.PriceCurrency,
		product.PriceAmount,
		product.Category,
		product.ID,
	).Scan(&product.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	return nil
}

// UpdateStock updates product stock quantity
func (r *ProductRepository) UpdateStock(ctx context.Context, productID string, delta int32) (int32, error) {
	query := `
		UPDATE products
		SET stock_quantity = stock_quantity + $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
		RETURNING stock_quantity
	`

	var newQuantity int32
	err := r.db.QueryRowContext(ctx, query, delta, productID).Scan(&newQuantity)
	if err != nil {
		return 0, fmt.Errorf("failed to update stock: %w", err)
	}

	return newQuantity, nil
}

// List retrieves products with pagination and filters
func (r *ProductRepository) List(ctx context.Context, limit int, cursor, category, searchQuery string) ([]*Product, string, error) {
	baseQuery := `
		SELECT id, name, description, sku, price_currency, price_amount, stock_quantity, category, image_urls, created_at, updated_at
		FROM products
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	// Apply category filter
	if category != "" {
		baseQuery += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	// Apply search filter
	if searchQuery != "" {
		baseQuery += fmt.Sprintf(" AND to_tsvector('english', name || ' ' || COALESCE(description, '')) @@ plainto_tsquery('english', $%d)", argIdx)
		args = append(args, searchQuery)
		argIdx++
	}

	// Apply cursor pagination
	if cursor != "" {
		baseQuery += fmt.Sprintf(" AND (created_at, id) < (SELECT created_at, id FROM products WHERE id = $%d)", argIdx)
		args = append(args, cursor)
		argIdx++
	}

	baseQuery += " ORDER BY created_at DESC, id DESC"
	baseQuery += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var products []*Product
	for rows.Next() {
		var product Product
		var imageURLs pq.StringArray

		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.SKU,
			&product.PriceCurrency,
			&product.PriceAmount,
			&product.StockQuantity,
			&product.Category,
			&imageURLs,
			&product.CreatedAt,
			&product.UpdatedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan product: %w", err)
		}

		product.ImageURLs = imageURLs
		products = append(products, &product)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows error: %w", err)
	}

	// Determine next cursor
	var nextCursor string
	if len(products) > limit {
		nextCursor = products[limit-1].ID
		products = products[:limit]
	}

	return products, nextCursor, nil
}

// CheckAvailability checks if products have sufficient stock
func (r *ProductRepository) CheckAvailability(ctx context.Context, items map[string]int32) (map[string]int32, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Build query for multiple products
	productIDs := make([]string, 0, len(items))
	for id := range items {
		productIDs = append(productIDs, id)
	}

	query := `
		SELECT id, stock_quantity
		FROM products
		WHERE id = ANY($1)
	`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(productIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to check availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	available := make(map[string]int32)
	for rows.Next() {
		var id string
		var quantity int32
		if err := rows.Scan(&id, &quantity); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		available[id] = quantity
	}

	return available, nil
}
