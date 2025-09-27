USE `42Tokyo2508-db`;

ALTER TABLE orders
  ADD COLUMN product_name VARCHAR(255) NOT NULL DEFAULT '';

UPDATE orders o
JOIN products p ON o.product_id = p.product_id
SET o.product_name = p.name;
