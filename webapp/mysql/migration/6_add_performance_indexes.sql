-- ソートで利用されるカラムにインデックスを追加
CREATE INDEX idx_products_name_id ON products (name, product_id);
CREATE INDEX idx_products_value_id ON products (value, product_id);
CREATE INDEX idx_products_weight_id ON products (weight, product_id);

-- AGENT.mdの指摘に基づき、注文履歴用のインデックスも追加
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at);
CREATE INDEX idx_orders_user_product_shipped ON orders (user_id, product_id, shipped_status);
