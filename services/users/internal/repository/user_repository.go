package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User represents a user entity
type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	Phone        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Address represents an address entity
type Address struct {
	ID         string
	UserID     string
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string
	IsDefault  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// UserRepository handles user data access
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (id, email, password_hash, full_name, phone)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`

	user.ID = uuid.New().String()

	err := r.db.QueryRowContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.FullName,
		user.Phone,
	).Scan(&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, phone, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.Phone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, full_name, phone, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.Phone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET full_name = $1, phone = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		user.FullName,
		user.Phone,
		user.ID,
	).Scan(&user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// List retrieves users with pagination
func (r *UserRepository) List(ctx context.Context, limit int, cursor string) ([]*User, string, error) {
	query := `
		SELECT id, email, password_hash, full_name, phone, created_at, updated_at
		FROM users
		WHERE ($1 = '' OR (created_at, id) > (
			SELECT created_at, id FROM users WHERE id = $1
		))
		ORDER BY created_at, id
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, cursor, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*User
	for rows.Next() {
		var user User
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.FullName,
			&user.Phone,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows error: %w", err)
	}

	// Determine next cursor
	var nextCursor string
	if len(users) > limit {
		nextCursor = users[limit-1].ID
		users = users[:limit]
	}

	return users, nextCursor, nil
}
