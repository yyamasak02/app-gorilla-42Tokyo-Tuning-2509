package service

import (
	"sort"

	"backend/internal/model"
)

type knapsackItem struct {
	Weight int
	Value  int
	Orders []model.DeliveryOrder
}

type productGroup struct {
	productID int
	orders    []model.DeliveryOrder
}

func buildKnapsackItems(orders []model.DeliveryOrder, capacity int) []knapsackItem {
	if capacity <= 0 || len(orders) == 0 {
		return nil
	}

	grouped := make(map[int][]model.DeliveryOrder, len(orders))
	for _, order := range orders {
		if order.Weight <= 0 || order.Weight > capacity {
			continue
		}
		grouped[order.ProductID] = append(grouped[order.ProductID], order)
	}

	if len(grouped) == 0 {
		return nil
	}

	groups := make([]productGroup, 0, len(grouped))
	for productID, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			return list[i].OrderID < list[j].OrderID
		})
		groups = append(groups, productGroup{
			productID: productID,
			orders:    list,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].orders[0].OrderID < groups[j].orders[0].OrderID
	})

	items := make([]knapsackItem, 0, len(orders))
	for _, grp := range groups {
		ordersForProduct := grp.orders
		weight := ordersForProduct[0].Weight
		value := ordersForProduct[0].Value

		usable := len(ordersForProduct)
		maxCount := capacity / weight
		if maxCount < usable {
			usable = maxCount
		}
		if usable == 0 {
			continue
		}

		ordersForProduct = ordersForProduct[:usable]
		remaining := usable
		offset := 0
		chunkSize := 1

		for remaining > 0 {
			size := chunkSize
			if size > remaining {
				size = remaining
			}

			chunkOrders := make([]model.DeliveryOrder, size)
			copy(chunkOrders, ordersForProduct[offset:offset+size])
			items = append(items, knapsackItem{
				Weight: weight * size,
				Value:  value * size,
				Orders: chunkOrders,
			})

			offset += size
			remaining -= size
			chunkSize <<= 1
		}
	}

	return items
}
