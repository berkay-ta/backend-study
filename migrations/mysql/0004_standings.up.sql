CREATE TABLE standing_snapshots (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    league_id      BIGINT UNSIGNED NOT NULL,
    snapshot_week  INT UNSIGNED    NOT NULL,
    league_version BIGINT UNSIGNED NOT NULL,
    created_at     TIMESTAMP(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_snapshot_per_version (league_id, league_version),
    KEY idx_snapshot_league_week (league_id, snapshot_week),
    CONSTRAINT fk_snapshot_league
        FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE standing_rows (
    snapshot_id   BIGINT UNSIGNED NOT NULL,
    team_id       BIGINT UNSIGNED NOT NULL,
    position      INT UNSIGNED    NOT NULL,
    played        INT UNSIGNED    NOT NULL,
    won           INT UNSIGNED    NOT NULL,
    drawn         INT UNSIGNED    NOT NULL,
    lost          INT UNSIGNED    NOT NULL,
    goals_for     INT UNSIGNED    NOT NULL,
    goals_against INT UNSIGNED    NOT NULL,
    goal_diff     INT             NOT NULL,
    points        INT UNSIGNED    NOT NULL,
    PRIMARY KEY (snapshot_id, team_id),
    CONSTRAINT fk_standing_rows_snapshot
        FOREIGN KEY (snapshot_id) REFERENCES standing_snapshots(id) ON DELETE CASCADE,
    CONSTRAINT fk_standing_rows_team
        FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
