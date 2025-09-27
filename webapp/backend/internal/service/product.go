package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ProductService struct {
	store        *repository.Store
	planCache    *cache.Cache
	productCache *cache.Cache
}

func NewProductService(store *repository.Store, planCache *cache.Cache, productCache *cache.Cache) *ProductService {
	return &ProductService{store: store, planCache: planCache, productCache: productCache}
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

		// まとめてオーダーを構築
		for _, item := range items {
			if item.Quantity <= 0 {
				continue
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
	if s.productCache != nil {
		s.productCache.Flush()
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

	cacheKey := fmt.Sprintf("products:u%d:q%s:p%d:ps%d:sf%s:so%s", userID, req.Search, req.Page, req.PageSize, req.SortField, req.SortOrder)
	if s.productCache != nil {
		if cached, found := s.productCache.Get(cacheKey); found {
			if entry, ok := cached.(struct {
				Products []model.Product
				Total    int
			}); ok {
				span.SetAttributes(attribute.String("cache.hit", "product"))
				return entry.Products, entry.Total, nil
			}
			s.productCache.Delete(cacheKey)
		}
	}

	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	if err != nil {
		span.RecordError(err)
	}

	if s.productCache != nil && err == nil {
		s.productCache.Set(cacheKey, struct {
			Products []model.Product
			Total    int
		}{Products: products, Total: total}, 5*time.Second)
	}

	return products, total, err
}
