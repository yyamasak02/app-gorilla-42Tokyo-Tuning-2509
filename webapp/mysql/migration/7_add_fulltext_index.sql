USE `42tokyo2508-db`;
CREATE FULLTEXT INDEX ft_products_name_description ON products(name, description);
