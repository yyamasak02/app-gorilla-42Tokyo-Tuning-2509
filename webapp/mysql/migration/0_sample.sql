-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
USE `42Tokyo2508-db`;
CREATE INDEX idx_products_name_pid
ON products(name, product_id, value, weight, image, description);


