-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
USE `42Tokyo2508-db`;
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_products_description ON products(description(255));
CREATE INDEX idx_products_name_pid ON products(name, product_id);
CREATE INDEX idx_products_value_pid ON products(value, product_id);
CREATE INDEX idx_products_weight_pid ON products(weight, product_id);
