package service

import (
	"strconv"
	"sync"
	"time"

	"backend/internal/model"
)

type cachedDeliveryPlan struct {
	plan      model.DeliveryPlan
	expiresAt time.Time
}

// DeliveryPlanCache keeps short-lived delivery plans keyed by robot and capacity.
type DeliveryPlanCache struct {
	mu      sync.RWMutex
	entries map[string]cachedDeliveryPlan
	ttl     time.Duration
}

// NewDeliveryPlanCache creates a cache with the provided TTL.
func NewDeliveryPlanCache(ttl time.Duration) *DeliveryPlanCache {
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	return &DeliveryPlanCache{
		entries: make(map[string]cachedDeliveryPlan),
		ttl:     ttl,
	}
}

func (c *DeliveryPlanCache) key(robotID string, capacity int) string {
	return robotID + ":" + strconv.Itoa(capacity)
}

func clonePlan(plan model.DeliveryPlan) model.DeliveryPlan {
	clone := plan
	if len(plan.Orders) > 0 {
		orders := make([]model.Order, len(plan.Orders))
		copy(orders, plan.Orders)
		clone.Orders = orders
	}
	return clone
}

// Get returns a copy of the cached delivery plan if it has not expired.
func (c *DeliveryPlanCache) Get(robotID string, capacity int) (*model.DeliveryPlan, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[c.key(robotID, capacity)]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}

	planCopy := clonePlan(entry.plan)
	return &planCopy, true
}

// Set caches a copy of the plan.
func (c *DeliveryPlanCache) Set(robotID string, capacity int, plan model.DeliveryPlan) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[c.key(robotID, capacity)] = cachedDeliveryPlan{
		plan:      clonePlan(plan),
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate drops the cached plan for the given robot/capacity pair.
func (c *DeliveryPlanCache) Invalidate(robotID string, capacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, c.key(robotID, capacity))
}

// InvalidateAll clears the entire cache.
func (c *DeliveryPlanCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]cachedDeliveryPlan)
}
