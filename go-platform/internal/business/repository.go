package business

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListApps(ctx context.Context, status string) ([]App, error) {
	where := "WHERE deleted_at IS NULL"
	args := []any{}
	if status != "" {
		where += " AND status = $" + strconv.Itoa(len(args)+1)
		args = append(args, status)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, code, name, description, icon, sort_order, status, created_at, updated_at
		 FROM business_apps `+where+` ORDER BY sort_order, code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var app App
		if err := rows.Scan(&app.ID, &app.Code, &app.Name, &app.Description, &app.Icon, &app.SortOrder, &app.Status, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}
