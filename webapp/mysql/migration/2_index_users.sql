-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
USE `42Tokyo2508-db`;
CREATE UNIQUE INDEX idx_users_user_name ON users(user_name);