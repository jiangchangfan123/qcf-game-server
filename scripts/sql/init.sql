-- ============================================
-- QCF Game Server 数据库初始化脚本
-- 使用方式: mysql -u root -p < scripts/sql/init.sql
-- ============================================

-- 1. 创建数据库
CREATE DATABASE IF NOT EXISTS game_server
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_general_ci;

USE game_server;

-- 2. 创建用户表
CREATE TABLE IF NOT EXISTS users (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username   VARCHAR(64)  NOT NULL UNIQUE COMMENT '用户名',
    password   VARCHAR(128) NOT NULL COMMENT '密码',
    nickname   VARCHAR(64)  DEFAULT '' COMMENT '昵称',
    created_at DATETIME     DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 3. 创建数据库用户并授权
CREATE USER IF NOT EXISTS 'jcf'@'localhost' IDENTIFIED BY 'jcf310978155';
GRANT ALL PRIVILEGES ON game_server.* TO 'jcf'@'localhost';
FLUSH PRIVILEGES;

-- 完成
SELECT '数据库初始化完成!' AS status;
