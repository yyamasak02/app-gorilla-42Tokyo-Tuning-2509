package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
	"sort"
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

func selectOrdersForDelivery(ctx context.Context, orders []model.DeliveryOrder, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "selectOrdersForDelivery")
	defer span.End()
	span.SetAttributes(
		attribute.String("robot.id", robotID),
	)
	span.SetAttributes(attribute.Int("orders.input.count", len(orders)))

	type aggregatedItem struct {
		weight int
		value  int
		orders []model.DeliveryOrder
	}

	grouped := make(map[int][]model.DeliveryOrder)
	for _, order := range orders {
		grouped[order.ProductID] = append(grouped[order.ProductID], order)
	}

	productIDs := make([]int, 0, len(grouped))
	for productID := range grouped {
		productIDs = append(productIDs, productID)
	}
	sort.Ints(productIDs)

	aggregated := make([]aggregatedItem, 0, len(orders))
	for _, productID := range productIDs {
		productOrders := grouped[productID]
		sort.Slice(productOrders, func(i, j int) bool {
			return productOrders[i].OrderID < productOrders[j].OrderID
		})

		if len(productOrders) == 0 {
			continue
		}

		unitWeight := productOrders[0].Weight
		if unitWeight <= 0 {
			for _, o := range productOrders {
				aggregated = append(aggregated, aggregatedItem{
					weight: o.Weight,
					value:  o.Value,
					orders: []model.DeliveryOrder{o},
				})
			}
			continue
		}
		maxChunkByCapacity := robotCapacity / unitWeight
		if maxChunkByCapacity == 0 {
			// 1 件でも積めない重量ならスキップ
			continue
		}

		remaining := len(productOrders)
		index := 0
		chunkSize := 1
		for remaining > 0 {
			actualChunk := chunkSize
			if actualChunk > remaining {
				actualChunk = remaining
			}
			if actualChunk > maxChunkByCapacity {
				actualChunk = maxChunkByCapacity
			}
			if actualChunk == 0 {
				break
			}

			ordersChunk := append([]model.DeliveryOrder(nil), productOrders[index:index+actualChunk]...)
			weight := 0
			value := 0
			for _, o := range ordersChunk {
				weight += o.Weight
				value += o.Value
			}

			aggregated = append(aggregated, aggregatedItem{
				weight: weight,
				value:  value,
				orders: ordersChunk,
			})

			index += actualChunk
			remaining -= actualChunk
			if remaining == 0 {
				break
			}

			if chunkSize < remaining {
				nextSize := chunkSize * 2
				if nextSize > maxChunkByCapacity {
					nextSize = maxChunkByCapacity
				}
				if nextSize == 0 {
					nextSize = 1
				}
				chunkSize = nextSize
			}
		}
	}

	// dp[w]: 容量wまでで得られる最大価値
	dp := make([]int, robotCapacity+1)

	// 選んだ注文のインデックスを保持するスライスのスライス
	keepTrack := make([][]int, robotCapacity+1)

	steps := 0
	checkEvery := 16384

	for i, item := range aggregated {
		if item.weight > robotCapacity {
			continue
		}
		for w := robotCapacity; w >= item.weight; w-- {
			steps++
			if checkEvery > 0 && steps%checkEvery == 0 {
				select {
				case <-ctx.Done():
					return model.DeliveryPlan{}, ctx.Err()
				default:
				}
			}

			if dp[w-item.weight]+item.value > dp[w] {
				dp[w] = dp[w-item.weight] + item.value

				newSet := make([]int, len(keepTrack[w-item.weight]))
				copy(newSet, keepTrack[w-item.weight])
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
	selectedOrders := make([]model.Order, 0, len(selectedIndexes))
	totalWeight := 0
	span.SetAttributes(attribute.Int("aggregated.items", len(aggregated)))
	for _, idx := range selectedIndexes {
		aggregatedItem := aggregated[idx]
		for _, order := range aggregatedItem.orders {
			selectedOrders = append(selectedOrders, model.Order{
				OrderID: order.OrderID,
				Weight:  order.Weight,
				Value:   order.Value,
				// 他のフィールドは未取得なのでゼロ値/NULLのまま
			})
			totalWeight += order.Weight
		}
	}
	span.SetAttributes(
		attribute.Int("plan.orders.count", len(selectedOrders)),
		attribute.Int("plan.totalWeight", totalWeight),
		attribute.Int("plan.totalValue", maxValue),
	)

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  maxValue,
		Orders:      selectedOrders, // []model.Order
	}, nil
}
