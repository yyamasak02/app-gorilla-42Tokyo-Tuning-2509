package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES (?, ?, 'shipping', NOW())`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID)
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

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

// 注文履歴一覧を取得
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	type orderRow struct {
		OrderID       int          `db:"order_id"`
		ProductID     int          `db:"product_id"`
		ProductName   string       `db:"product_name"`
		ShippedStatus string       `db:"shipped_status"`
		CreatedAt     sql.NullTime `db:"created_at"`
		ArrivedAt     sql.NullTime `db:"arrived_at"`
	}

	var rows []orderRow

	baseQuery := `
		SELECT o.order_id, o.product_id, p.name AS product_name,
		       o.shipped_status, o.created_at, o.arrived_at
		FROM orders o
		JOIN products p ON o.product_id = p.product_id
		WHERE o.user_id = ?
	`
	args := []interface{}{userID}
	whereClause := ""

	if req.Search != "" {
		if req.Type == "prefix" {
			whereClause = " AND p.name LIKE ?"
			args = append(args, req.Search+"%")
		} else {
			whereClause = " AND p.name LIKE ?"
			args = append(args, "%"+req.Search+"%")
		}
	}

	// バリデーション付きでソートフィールド選定
	sortFieldMap := map[string]string{
		"product_name":   "p.name",
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
		return nil, 0, err
	}

	// 総件数取得
	var total int
	countQuery := "SELECT COUNT(*) FROM orders o JOIN products p ON o.product_id = p.product_id WHERE o.user_id = ?" + whereClause
	countArgs := args[:len(args)-2] // LIMIT/OFFSETを除外
	if err := r.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
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
			CreatedAt:     o.CreatedAt.Time,
			ArrivedAt:     o.ArrivedAt,
		})
	}

	return orders, total, nil
}
