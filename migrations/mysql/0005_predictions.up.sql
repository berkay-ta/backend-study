CREATE TABLE prediction_runs (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    league_id      BIGINT UNSIGNED NOT NULL,
    strategy       ENUM('monte_carlo','ai_analyst') NOT NULL,
    snapshot_id    BIGINT UNSIGNED NOT NULL,
    league_version BIGINT UNSIGNED NOT NULL,
    created_at     TIMESTAMP(3)    NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    params_json    JSON            NULL,
    notes          TEXT            NULL,
    stale          BOOLEAN         NOT NULL DEFAULT FALSE,
    PRIMARY KEY (id),
    KEY idx_prediction_league_created (league_id, created_at),
    CONSTRAINT fk_prediction_league
        FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    CONSTRAINT fk_prediction_snapshot
        FOREIGN KEY (snapshot_id) REFERENCES standing_snapshots(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE prediction_percentages (
    run_id                 BIGINT UNSIGNED NOT NULL,
    team_id                BIGINT UNSIGNED NOT NULL,
    projected_position     INT UNSIGNED    NOT NULL,
    championship_pct       DECIMAL(5,2)    NOT NULL,
    average_position       DECIMAL(5,2)    NOT NULL,
    played                 INT UNSIGNED    NOT NULL,
    expected_won           DECIMAL(6,2)    NOT NULL,
    expected_drawn         DECIMAL(6,2)    NOT NULL,
    expected_lost          DECIMAL(6,2)    NOT NULL,
    expected_goals_for     DECIMAL(6,2)    NOT NULL,
    expected_goals_against DECIMAL(6,2)    NOT NULL,
    expected_goal_diff     DECIMAL(6,2)    NOT NULL,
    expected_points        DECIMAL(6,2)    NOT NULL,
    PRIMARY KEY (run_id, team_id),
    KEY idx_prediction_table_order (run_id, projected_position),
    CONSTRAINT fk_pct_run
        FOREIGN KEY (run_id) REFERENCES prediction_runs(id) ON DELETE CASCADE,
    CONSTRAINT fk_pct_team
        FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
