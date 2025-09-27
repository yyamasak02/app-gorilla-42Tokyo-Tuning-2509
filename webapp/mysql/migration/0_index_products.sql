-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
USE `42Tokyo2508-db`;
ALTER TABLE products 
  ADD FULLTEXT INDEX idx_products_fulltext (name, description) WITH PARSER ngram;
CREATE INDEX idx_products_name_pid ON products(name, product_id);
CREATE INDEX idx_products_value_pid ON products(value, product_id);
CREATE INDEX idx_products_weight_pid ON products(weight, product_id);
