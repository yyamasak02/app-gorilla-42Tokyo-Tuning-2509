package service

import (
	"context"
	"fmt"
	"log"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ProductService struct {
	store     *repository.Store
	planCache *cache.Cache
}

func NewProductService(store *repository.Store, planCache *cache.Cache) *ProductService {
	return &ProductService{store: store, planCache: planCache}
}

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "CreateOrders")
	defer span.End()
	span.SetAttributes(attribute.Int("user.id", userID), attribute.Int("items.count", len(items)))
	span.SetAttributes(attribute.Int("items.nonEmpty", countPositiveQuantities(items)))

	var insertedOrderIDs []string

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		var orders []*model.Order
		productIDs := make([]int, 0)
		productSeen := make(map[int]struct{})

		// まとめてオーダーを構築
		for _, item := range items {
			if item.Quantity <= 0 {
				continue
			}
			if _, ok := productSeen[item.ProductID]; !ok {
				productIDs = append(productIDs, item.ProductID)
				productSeen[item.ProductID] = struct{}{}
			}
			for i := 0; i < item.Quantity; i++ {
				orders = append(orders, &model.Order{
					UserID:    userID,
					ProductID: item.ProductID,
				})
			}
		}

		orderCount := len(orders)
		span.SetAttributes(attribute.Int("orders.toInsert", orderCount))
		if orderCount == 0 {
			span.AddEvent("no_orders_to_insert")
			return nil
		}

		weights, err := txStore.ProductRepo.FetchWeightsAndValues(ctx, productIDs)
		if err != nil {
			return err
		}
		names, err := txStore.ProductRepo.FetchProductNames(ctx, productIDs)
		if err != nil {
			return err
		}
		for _, order := range orders {
			wv, ok := weights[order.ProductID]
			if !ok {
				return fmt.Errorf("product %d not found during order creation", order.ProductID)
			}
			name, ok := names[order.ProductID]
			if !ok {
				return fmt.Errorf("product %d name not found during order creation", order.ProductID)
			}
			order.Weight = wv.Weight
			order.Value = wv.Value
			order.ProductName = name
		}

		// バルクINSERTに対応したリポジトリメソッドを利用
		ids, err := txStore.OrderRepo.CreateBulk(ctx, orders)
		if err != nil {
			return err
		}
		insertedOrderIDs = ids
		return nil
	})

	if err != nil {
		return nil, err
	}
	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	if s.planCache != nil {
		span.AddEvent("flushing_delivery_plan_cache")
		s.planCache.Flush()
	}
	return insertedOrderIDs, nil
}

func countPositiveQuantities(items []model.RequestItem) int {
	count := 0
	for _, item := range items {
		if item.Quantity > 0 {
			count++
		}
	}
	return count
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "FetchProducts")
	defer span.End()
	span.SetAttributes(
		attribute.Int("user.id", userID),
		attribute.Int("page", req.Page),
		attribute.Int("pageSize", req.PageSize),
	)

	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	if err != nil {
		span.RecordError(err)
	}
	return products, total, err
}
