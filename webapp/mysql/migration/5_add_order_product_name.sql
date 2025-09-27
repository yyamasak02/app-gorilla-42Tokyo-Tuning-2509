USE `42tokyo2508-db`;

ALTER TABLE orders
ADD COLUMN product_name VARCHAR(255);

DELIMITER //
CREATE PROCEDURE BatchUpdateOrdersName()
BEGIN
    DECLARE updated_rows INT;
    REPEAT
        UPDATE orders o
        JOIN products p ON o.product_id = p.product_id
        SET o.product_name = p.name
        WHERE o.product_name IS NULL
        LIMIT 10000;
        
        SELECT ROW_COUNT() INTO updated_rows;

    UNTIL updated_rows = 0 END REPEAT;
END //
DELIMITER ;

CALL BatchUpdateOrdersName();
DROP PROCEDURE BatchUpdateOrdersName();