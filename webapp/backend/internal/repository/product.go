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

// 商品一覧をデータベース側でページング処理を行う
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var products []model.Product
	var total int
	
	// 総件数を取得するクエリ
	countQuery := `SELECT COUNT(*) FROM products`
	countArgs := []interface{}{}
	
	if req.Search != "" {
		countQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		countArgs = append(countArgs, searchPattern, searchPattern)
	}
	
	err := r.db.GetContext(ctx, &total, countQuery, countArgs...)
	if err != nil {
		return nil, 0, err
	}
	
	// 商品データを取得するクエリ（ページング付き）
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}
	
	if req.Search != "" {
		baseQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}
	
	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)
	
	err = r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	
	return products, total, nil
}
