package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
	"strconv"

	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type RobotService struct {
	store     *repository.Store
	planCache *cache.Cache
}

func NewRobotService(store *repository.Store, planCache *cache.Cache) *RobotService {
	return &RobotService{store: store, planCache: planCache}
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	cacheKey := robotID + ":" + strconv.Itoa(capacity)
	if s.planCache != nil {
		if cachedPlan, found := s.planCache.Get(cacheKey); found {
			if plan, ok := cachedPlan.(model.DeliveryPlan); ok {
				planCopy := cloneDeliveryPlan(plan)
				return &planCopy, nil
			}
			// 型が想定と異なる場合は安全のため削除
			s.planCache.Delete(cacheKey)
		}
	}

	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "GenerateDeliveryPlan")
	defer span.End()
	// スパン属性を追加（引数ベース）
	span.SetAttributes(
		attribute.String("robot.id", robotID),
		attribute.Int("capacity", capacity),
	)
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx, capacity)
			if err != nil {
				return err
			}
			span.SetAttributes(attribute.Int("orders.shipping.count", len(orders)))
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	if len(plan.Orders) == 0 {
		span.SetAttributes(attribute.Bool("delivery_plan.empty", true))
		span.AddEvent("delivery_plan_empty")
	}

	if s.planCache != nil {
		planCopy := cloneDeliveryPlan(plan)
		s.planCache.Set(cacheKey, planCopy, cache.DefaultExpiration)
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		if err := s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus); err != nil {
			return err
		}
		if s.planCache != nil {
			s.planCache.Flush()
		}
		return nil
	})
}

func cloneDeliveryPlan(plan model.DeliveryPlan) model.DeliveryPlan {
	ordersCopy := make([]model.Order, len(plan.Orders))
	copy(ordersCopy, plan.Orders)
	return model.DeliveryPlan{
		RobotID:     plan.RobotID,
		TotalWeight: plan.TotalWeight,
		TotalValue:  plan.TotalValue,
		Orders:      ordersCopy,
	}
}

func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "selectOrdersForDelivery")
	defer span.End()
	span.SetAttributes(
		attribute.String("robot.id", robotID),
	)
	span.SetAttributes(attribute.Int("orders.input.count", len(orders)))

	trimmed := make([]model.Order, len(orders))
	for i, o := range orders {
		trimmed[i] = model.Order{OrderID: o.OrderID, Weight: o.Weight, Value: o.Value}
	}

	type memoKey struct {
		idx int
		cap int
	}

	type memoEntry struct {
		value  int
		orders []model.Order
	}

	memo := make(map[memoKey]memoEntry)
	steps := 0
	checkEvery := 16384

	var dfs func(int, int) (int, []model.Order, bool)
	dfs = func(i int, remaining int) (int, []model.Order, bool) {
		if remaining < 0 {
			return -1, nil, false
		}
		steps++
		if checkEvery > 0 && steps%checkEvery == 0 {
			select {
			case <-ctx.Done():
				return 0, nil, true
			default:
			}
		}

		if i == len(trimmed) {
			return 0, nil, false
		}

		key := memoKey{idx: i, cap: remaining}
		if entry, ok := memo[key]; ok {
			return entry.value, append([]model.Order(nil), entry.orders...), false
		}

		bestValue := 0
		bestOrders := []model.Order{}

		skipValue, skipOrders, canceled := dfs(i+1, remaining)
		if canceled {
			return 0, nil, true
		}
		bestValue = skipValue
		bestOrders = append(bestOrders, skipOrders...)

		order := trimmed[i]
		if order.Weight <= remaining {
			takeValue, takeOrders, canceled := dfs(i+1, remaining-order.Weight)
			if canceled {
				return 0, nil, true
			}
			takeValue += order.Value
			if takeValue > bestValue {
				bestValue = takeValue
				bestOrders = append([]model.Order{order}, takeOrders...)
			}
		}

		memo[key] = memoEntry{value: bestValue, orders: append([]model.Order(nil), bestOrders...)}
		return bestValue, append([]model.Order(nil), bestOrders...), false
	}

	bestValue, bestOrders, canceled := dfs(0, robotCapacity)
	if canceled {
		return model.DeliveryPlan{}, ctx.Err()
	}

	var totalWeight int
	for _, o := range bestOrders {
		totalWeight += o.Weight
	}

	span.SetAttributes(
		attribute.Int("plan.orders.count", len(bestOrders)),
		attribute.Int("plan.totalWeight", totalWeight),
		attribute.Int("plan.totalValue", bestValue),
	)

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      bestOrders,
	}, nil
}
