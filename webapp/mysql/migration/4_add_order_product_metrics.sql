USE `42Tokyo2508-db`;

ALTER TABLE orders
  ADD COLUMN product_weight INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN product_value INT UNSIGNED NOT NULL DEFAULT 0;

UPDATE orders o
JOIN products p ON o.product_id = p.product_id
SET o.product_weight = p.weight,
    o.product_value = p.value;
