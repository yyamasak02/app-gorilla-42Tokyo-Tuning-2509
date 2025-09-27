package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type RobotService struct {
	store     *repository.Store
	planCache *DeliveryPlanCache
}

func NewRobotService(store *repository.Store, cache *DeliveryPlanCache) *RobotService {
	return &RobotService{store: store, planCache: cache}
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "GenerateDeliveryPlan")
	defer span.End()
	// スパン属性を追加（引数ベース）
	span.SetAttributes(
		attribute.String("robot.id", robotID),
		attribute.Int("capacity", capacity),
	)
	if s.planCache != nil {
		if cached, ok := s.planCache.Get(robotID, capacity); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return cached, nil
		}
	}

	span.SetAttributes(attribute.Bool("cache.hit", false))
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx, capacity)
			if err != nil {
				return err
			}
			items := buildKnapsackItems(orders, capacity)
			span.SetAttributes(
				attribute.Int("orders.raw", len(orders)),
				attribute.Int("orders.items", len(items)),
			)
			plan, err = selectOrdersForDelivery(ctx, items, robotID, capacity)
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
	if s.planCache != nil {
		s.planCache.Set(robotID, capacity, plan)
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus)
	})
	if err == nil && s.planCache != nil {
		// Delivery completion or status change means plans should be recomputed.
		s.planCache.InvalidateAll()
	}
	return err
}

func selectOrdersForDelivery(ctx context.Context, items []knapsackItem, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "selectOrdersForDelivery")
	defer span.End()
	span.SetAttributes(
		attribute.String("robot.id", robotID),
	)
	// dp[w]: 容量wまでで得られる最大価値
	dp := make([]int, robotCapacity+1)

	// 選んだ注文のインデックスを保持するスライスのスライス
	keepTrack := make([][]int, robotCapacity+1)

	steps := 0
	checkEvery := 16384

	for i, item := range items {
		for w := robotCapacity; w >= item.Weight; w-- {
			steps++
			if checkEvery > 0 && steps%checkEvery == 0 {
				select {
				case <-ctx.Done():
					return model.DeliveryPlan{}, ctx.Err()
				default:
				}
			}

			if dp[w-item.Weight]+item.Value > dp[w] {
				dp[w] = dp[w-item.Weight] + item.Value

				// 選んだ注文の更新（新規コピーにして追加）
				newSet := make([]int, len(keepTrack[w-item.Weight]))
				copy(newSet, keepTrack[w-item.Weight])
				newSet = append(newSet, i)
				keepTrack[w] = newSet
			}
		}
	}

	// 最大価値と対応する注文セットを特定
	maxValue := 0
	maxIndex := 0
	for w, val := range dp {
		if val > maxValue {
			maxValue = val
			maxIndex = w
		}
	}

	// 注文を復元 & DeliveryOrder → Order に変換
	selectedIndexes := keepTrack[maxIndex]
	selectedOrders := make([]model.Order, 0)
	totalWeight := 0
	for _, idx := range selectedIndexes {
		chunk := items[idx]
		for _, d := range chunk.Orders {
			selectedOrders = append(selectedOrders, model.Order{
				OrderID: d.OrderID,
				Weight:  d.Weight,
				Value:   d.Value,
				// 他のフィールドは未取得なのでゼロ値/NULLのまま
			})
			totalWeight += d.Weight
		}
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  maxValue,
		Orders:      selectedOrders, // []model.Order
	}, nil
}
