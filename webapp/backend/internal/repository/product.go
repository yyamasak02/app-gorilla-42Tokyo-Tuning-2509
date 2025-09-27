package repository

import (
	"backend/internal/model"
	"context"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧を全件取得し、アプリケーション側でページング処理を行う
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var products []model.Product

	// 1. データ取得クエリ（LIMIT + OFFSET）
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}
	whereClause := ""

	if req.Search != "" {
		whereClause = " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// 安全なソートフィールドと順序をバリデーション
	sortField := req.SortField
	sortOrder := req.SortOrder
	if sortField != "value" && sortField != "weight" && sortField != "name" {
		sortField = "product_id"
	}
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "ASC"
	}

	orderClause := " ORDER BY " + sortField + " " + sortOrder + ", product_id ASC"
	limitOffset := " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	query := baseQuery + whereClause + orderClause + limitOffset

	err := r.db.SelectContext(ctx, &products, query, args...)
	if err != nil {
		return nil, 0, err
	}

	// 2. 総件数の取得（COUNT）
	var total int
	countQuery := "SELECT COUNT(*) FROM products" + whereClause
	countArgs := args[:len(args)-2] // LIMIT/OFFSETは外す
	err = r.db.GetContext(ctx, &total, countQuery, countArgs...)
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}
