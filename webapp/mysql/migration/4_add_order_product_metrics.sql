USE `42tokyo2508-db`;

ALTER TABLE orders
ADD COLUMN product_weight INT,
ADD COLUMN product_value INT;

DELIMITER //
CREATE PROCEDURE BatchUpdateOrdersMetrics()
BEGIN
    DECLARE updated_rows INT;
    REPEAT
        UPDATE orders o
        JOIN products p ON o.product_id = p.product_id
        SET o.product_weight = p.weight,
            o.product_value = p.value
        WHERE o.product_weight IS NULL
        LIMIT 10000;
        
        SELECT ROW_COUNT() INTO updated_rows;
        
    UNTIL updated_rows = 0 END REPEAT;
END //
DELIMITER ;

CALL BatchUpdateOrdersMetrics();
DROP PROCEDURE BatchUpdateOrdersMetrics();