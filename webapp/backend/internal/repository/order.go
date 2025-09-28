package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// bulk insert
func (r *OrderRepository) CreateBulk(ctx context.Context, orders []*model.Order) ([]string, error) {
	if len(orders) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(orders))
	vals := make([]interface{}, 0, len(orders)*5)

	for i, o := range orders {
		placeholders[i] = "(?, ?, 'shipping', NOW(), ?, ?, ?)"
		vals = append(vals, o.UserID, o.ProductID, o.ProductName, o.Weight, o.Value)
	}

	query := "INSERT INTO orders (user_id, product_id, shipped_status, created_at, product_name, product_weight, product_value) VALUES " +
		strings.Join(placeholders, ",")

	result, err := r.db.ExecContext(ctx, query, vals...)
	if err != nil {
		return nil, err
	}

	// AUTO_INCREMENT の最初の ID を取得
	firstID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(orders))
	for i := range orders {
		ids[i] = fmt.Sprintf("%d", firstID+int64(i))
	}

	return ids, nil
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at, product_name, product_weight, product_value) VALUES (?, ?, 'shipping', NOW(), ?, ?, ?)`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID, order.ProductName, order.Weight, order.Value)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "UpdateStatuses")
	defer span.End()
	span.SetAttributes(
		attribute.String("newStatus", newStatus),
	)
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *OrderRepository) GetShippingOrders(ctx context.Context, capacity int) ([]model.Order, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "GetShippingOrders")
	defer span.End()

	var orders []model.Order
	query := `
		SELECT
			order_id,
			product_id,
			product_name,
			product_weight,
			product_value
		FROM orders
		WHERE shipped_status = 'shipping'
		AND product_weight <= ?
		ORDER BY product_value DESC, product_weight ASC, order_id ASC;
	`
	err := r.db.SelectContext(ctx, &orders, query, capacity)

	span.SetAttributes(
		attribute.String("zokusei", "GetShippingOrders"),
		attribute.Int("capacity", capacity),
		attribute.Int("orders.length", len(orders)),
	)

	return orders, err
}

// 注文履歴一覧を取得
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "ListOrders")
	defer span.End()
	type orderRow struct {
		OrderID       int          `db:"order_id"`
		ProductID     int          `db:"product_id"`
		ProductName   string       `db:"product_name"`
		ShippedStatus string       `db:"shipped_status"`
		ProductWeight int          `db:"product_weight"`
		ProductValue  int          `db:"product_value"`
		CreatedAt     sql.NullTime `db:"created_at"`
		ArrivedAt     sql.NullTime `db:"arrived_at"`
	}

	var rows []orderRow

	baseQuery := `
		SELECT 
			o.order_id, 
			o.product_id, 
			o.product_name,
		    o.shipped_status,
			o.product_weight,
			o.product_value,
			o.created_at, 
			o.arrived_at
		FROM 
			orders o
		WHERE 1 = 1
		AND o.user_id = ?
	`
	args := []interface{}{userID}
	whereClause := ""

	if req.Search != "" {
		whereClause = " AND o.product_name LIKE ?"
		if req.Type == "prefix" {
			args = append(args, req.Search+"%")
		} else {
			args = append(args, "%"+req.Search+"%")
		}
	}

	// バリデーション付きでソートフィールド選定
	sortFieldMap := map[string]string{
		"product_name":   "o.product_name",
		"created_at":     "o.created_at",
		"shipped_status": "o.shipped_status",
		"arrived_at":     "o.arrived_at",
		"order_id":       "o.order_id",
	}

	sortField, ok := sortFieldMap[req.SortField]
	if !ok {
		sortField = "o.order_id"
	}

	sortOrder := strings.ToUpper(req.SortOrder)
	if sortOrder != "DESC" {
		sortOrder = "ASC"
	}

	orderClause := fmt.Sprintf(" ORDER BY %s %s, o.order_id ASC", sortField, sortOrder)

	limitOffset := " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	// 最終クエリ組み立て
	query := baseQuery + whereClause + orderClause + limitOffset

	// データ取得
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		span.SetAttributes(
			attribute.Int("user.id", userID),
			attribute.String("place", "r.db.SelectContext"),
			attribute.String("error", err.Error()),
		)
		return nil, 0, err
	}

	// 総件数取得
	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM orders o
		WHERE 1=1
		AND o.user_id = ?
	` + strings.ReplaceAll(whereClause, "p.name", "o.product_name")
	countArgs := args[:len(args)-2] // LIMIT/OFFSETを除外
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		span.SetAttributes(
			attribute.Int("user.id", userID),
			attribute.String("place", "r.db.GetContext"),
			attribute.String("error", err.Error()),
		)
		return nil, 0, err
	}

	// 結果マッピング
	var orders []model.Order
	for _, o := range rows {
		orders = append(orders, model.Order{
			OrderID:       int64(o.OrderID),
			ProductID:     o.ProductID,
			ProductName:   o.ProductName,
			ShippedStatus: o.ShippedStatus,
			Weight:        o.ProductWeight,
			Value:         o.ProductValue,
			CreatedAt:     o.CreatedAt.Time,
			ArrivedAt:     o.ArrivedAt,
		})
	}

	span.SetAttributes(
		attribute.Int("user.id", userID),
		attribute.Int("total", total),
	)

	return orders, total, nil
}
