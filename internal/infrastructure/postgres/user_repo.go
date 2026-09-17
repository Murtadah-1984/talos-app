package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/talos-platform/talos-platform/internal/domain/shared"
	"github.com/talos-platform/talos-platform/internal/domain/user"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	u.Touch()
	if u.ID == (shared.ID{}) {
		u.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, name, subject, disabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.Name, u.Subject, u.Disabled, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}
	return nil
}

func (r *UserRepository) Get(ctx context.Context, id shared.ID) (*user.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, name, subject, disabled, created_at, updated_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *UserRepository) GetBySubject(ctx context.Context, subject string) (*user.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, name, subject, disabled, created_at, updated_at FROM users WHERE subject = $1`, subject)
	return scanUser(row)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, name, subject, disabled, created_at, updated_at FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (r *UserRepository) List(ctx context.Context, page shared.Page) ([]*user.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, name, subject, disabled, created_at, updated_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var out []*user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	u.Touch()
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET email = $2, name = $3, disabled = $4, updated_at = $5 WHERE id = $1`,
		u.ID, u.Email, u.Name, u.Disabled, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *UserRepository) CreateRoleBinding(ctx context.Context, rb *user.RoleBinding) error {
	rb.Touch()
	if rb.ID == (shared.ID{}) {
		rb.ID = shared.NewID()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO role_bindings (id, user_id, role, resource_kind, resource_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rb.ID, rb.UserID, rb.Role, rb.ResourceKind, rb.ResourceID, rb.CreatedAt, rb.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting role binding: %w", err)
	}
	return nil
}

func (r *UserRepository) ListRoleBindingsForUser(ctx context.Context, userID shared.ID) ([]*user.RoleBinding, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, role, resource_kind, resource_id, created_at, updated_at FROM role_bindings WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("listing role bindings: %w", err)
	}
	defer rows.Close()

	var out []*user.RoleBinding
	for rows.Next() {
		rb := &user.RoleBinding{}
		var role, kind string
		if err := rows.Scan(&rb.ID, &rb.UserID, &role, &kind, &rb.ResourceID, &rb.CreatedAt, &rb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning role binding: %w", err)
		}
		rb.Role = user.Role(role)
		rb.ResourceKind = user.ResourceKind(kind)
		out = append(out, rb)
	}
	return out, rows.Err()
}

func (r *UserRepository) DeleteRoleBinding(ctx context.Context, id shared.ID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM role_bindings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting role binding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func scanUser(row rowScanner) (*user.User, error) {
	u := &user.User{}
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Subject, &u.Disabled, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return u, nil
}
