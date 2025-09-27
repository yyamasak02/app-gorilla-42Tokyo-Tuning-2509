package repository

import (
	"backend/internal/model"
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧をページング付きで取得し、総件数も返す
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "ListProducts")
	defer span.End()

	var products []model.Product
	var total int

	// クエリ共通部分
	baseQuery := "SELECT product_id, name, value, weight, image, description FROM products"
	args := []interface{}{}
	countArgs := []interface{}{}
	whereClause := ""
	if req.Search != "" {
		searchPattern := "%" + strings.TrimSpace(req.Search) + "%"
		whereClause = " WHERE (name LIKE ? OR description LIKE ?)"
		baseQuery += whereClause
		args = append(args, searchPattern, searchPattern)
		countArgs = append(countArgs, searchPattern, searchPattern)
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
	dataArgs := append(append([]interface{}{}, args...), req.PageSize, req.Offset)

	// 並列実行
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		query := baseQuery + orderClause + limitOffset
		return r.db.SelectContext(ctx, &products, query, dataArgs...)
	})

	g.Go(func() error {
		cnt, err := r.CountProducts(ctx, whereClause, countArgs)
		if err != nil {
			return err
		}
		total = cnt
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, 0, err
	}

	span.SetAttributes(
		attribute.String("SortField", sortField),
		attribute.String("SortOrder", sortOrder),
		attribute.Int("ReturnedCount", len(products)),
		attribute.Int("TotalCount", total),
	)

	return products, total, nil
}

// 総件数だけを取得する共通メソッド
func (r *ProductRepository) CountProducts(ctx context.Context, whereClause string, countArgs []interface{}) (int, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "CountProducts")
	defer span.End()
	var total int
	countQuery := "SELECT COUNT(*) FROM products" + whereClause
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		span.RecordError(err)
		return 0, err
	}
	span.SetAttributes(attribute.Int("products.count", total))
	return total, nil
}

func (r *ProductRepository) FetchProductNames(ctx context.Context, productIDs []int) (map[int]string, error) {
	if len(productIDs) == 0 {
		return map[int]string{}, nil
	}
	query, args, err := sqlx.In("SELECT product_id, name FROM products WHERE product_id IN (?)", productIDs)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)
	rows := []struct {
		ProductID int    `db:"product_id"`
		Name      string `db:"name"`
	}{}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[int]string, len(rows))
	for _, row := range rows {
		result[row.ProductID] = row.Name
	}
	return result, nil
}

func (r *ProductRepository) FetchWeightsAndValues(ctx context.Context, productIDs []int) (map[int]struct{ Weight, Value int }, error) {
	if len(productIDs) == 0 {
		return map[int]struct{ Weight, Value int }{}, nil
	}
	query, args, err := sqlx.In("SELECT product_id, weight, value FROM products WHERE product_id IN (?)", productIDs)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)
	rows := []struct {
		ProductID int `db:"product_id"`
		Weight    int `db:"weight"`
		Value     int `db:"value"`
	}{}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[int]struct{ Weight, Value int }, len(rows))
	for _, row := range rows {
		result[row.ProductID] = struct{ Weight, Value int }{Weight: row.Weight, Value: row.Value}
	}
	return result, nil
}
