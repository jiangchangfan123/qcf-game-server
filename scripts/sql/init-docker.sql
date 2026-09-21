-- QCF Game Server init
USE game_server;

CREATE TABLE IF NOT EXISTS users (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username   VARCHAR(64)  NOT NULL UNIQUE,
    password   VARCHAR(128) NOT NULL,
    nickname   VARCHAR(64)  DEFAULT '',
    created_at DATETIME     DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS game_records (
    id         BIGINT   NOT NULL AUTO_INCREMENT PRIMARY KEY,
    player1    BIGINT   NOT NULL,
    player2    BIGINT   NOT NULL,
    winner     BIGINT   NOT NULL DEFAULT 0,
    score1     INT      NOT NULL DEFAULT 0,
    score2     INT      NOT NULL DEFAULT 0,
    round      INT      NOT NULL DEFAULT 0,
    duration   INT      NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_player1 (player1),
    INDEX idx_player2 (player2),
    INDEX idx_winner (winner),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
