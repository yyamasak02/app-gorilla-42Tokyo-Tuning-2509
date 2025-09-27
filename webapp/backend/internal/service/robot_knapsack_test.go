package service

import (
	"context"
	"testing"

	"backend/internal/model"
)

func TestBuildKnapsackItemsRespectsCapacity(t *testing.T) {
	orders := make([]model.DeliveryOrder, 0, 10)
	for i := 0; i < 10; i++ {
		orders = append(orders, model.DeliveryOrder{
			OrderID:   int64(100 + i),
			ProductID: 1,
			Weight:    2,
			Value:     5,
		})
	}

	capacity := 5
	items := buildKnapsackItems(orders, capacity)
	if len(items) == 0 {
		t.Fatalf("expected items to be generated")
	}

	totalOrders := 0
	for _, item := range items {
		totalOrders += len(item.Orders)
		if item.Weight > capacity {
			t.Fatalf("item weight %d exceeds capacity %d", item.Weight, capacity)
		}
	}

	expected := capacity / 2
	if totalOrders != expected {
		t.Fatalf("expected %d orders to remain after pruning, got %d", expected, totalOrders)
	}
}

func TestSelectOrdersForDeliveryWithAggregatedItems(t *testing.T) {
	orders := []model.DeliveryOrder{
		{OrderID: 1, ProductID: 10, Weight: 2, Value: 6},
		{OrderID: 2, ProductID: 10, Weight: 2, Value: 6},
		{OrderID: 3, ProductID: 10, Weight: 2, Value: 6},
		{OrderID: 4, ProductID: 10, Weight: 2, Value: 6},
		{OrderID: 5, ProductID: 10, Weight: 2, Value: 6},
		{OrderID: 6, ProductID: 20, Weight: 3, Value: 9},
		{OrderID: 7, ProductID: 20, Weight: 3, Value: 9},
		{OrderID: 8, ProductID: 20, Weight: 3, Value: 9},
	}

	capacity := 10
	items := buildKnapsackItems(orders, capacity)
	if len(items) >= len(orders) {
		t.Fatalf("expected aggregated items to be fewer than raw orders (got %d >= %d)", len(items), len(orders))
	}

	plan, err := selectOrdersForDelivery(context.Background(), items, "robot-1", capacity)
	if err != nil {
		t.Fatalf("selectOrdersForDelivery returned error: %v", err)
	}

	if plan.TotalValue != 30 {
		t.Fatalf("expected total value 30, got %d", plan.TotalValue)
	}

	if plan.TotalWeight > capacity {
		t.Fatalf("plan exceeds capacity: weight=%d capacity=%d", plan.TotalWeight, capacity)
	}

	seen := make(map[int64]struct{})
	for _, o := range plan.Orders {
		if _, ok := seen[o.OrderID]; ok {
			t.Fatalf("duplicate order id %d in plan", o.OrderID)
		}
		seen[o.OrderID] = struct{}{}
	}

	for _, o := range plan.Orders {
		if o.Weight <= 0 {
			t.Fatalf("unexpected non-positive weight for order %d", o.OrderID)
		}
		found := false
		for _, src := range orders {
			if src.OrderID == o.OrderID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("order %d in plan not present in source orders", o.OrderID)
		}
	}
}
