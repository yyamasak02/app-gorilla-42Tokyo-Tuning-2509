USE `42tokyo2508-db`;

ALTER TABLE orders
ADD COLUMN product_name VARCHAR(255);

DELIMITER //
CREATE PROCEDURE BatchUpdateOrdersName()
BEGIN
    DECLARE updated_rows INT DEFAULT 1;
    WHILE updated_rows > 0 DO
        UPDATE orders o
        JOIN products p ON o.product_id = p.product_id
        SET o.product_name = p.name
        WHERE o.order_id IN (
            SELECT order_id FROM (
                SELECT order_id FROM orders WHERE product_name IS NULL LIMIT 10000
            ) as t
        );
        SELECT ROW_COUNT() INTO updated_rows;
    END WHILE;
END //
DELIMITER ;

CALL BatchUpdateOrdersName();
DROP PROCEDURE BatchUpdateOrdersName();
