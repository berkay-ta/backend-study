CREATE TABLE teams (
    id        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    league_id BIGINT UNSIGNED NOT NULL,
    name      VARCHAR(80)     NOT NULL,
    strength  TINYINT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_teams_league_name (league_id, name),
    KEY idx_teams_league (league_id),
    CONSTRAINT fk_teams_league
        FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
