-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
USE `42Tokyo2508-db`;
CREATE INDEX idx_orders_uid_pid
	ON orders(user_id, product_id, shipped_status);
-- orders の shipped_status + product_id + order_id に複合インデックス
CREATE INDEX idx_orders_status_product_orderid
   ON orders (shipped_status, product_id, order_id);