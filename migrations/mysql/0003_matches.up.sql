CREATE TABLE matches (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    league_id    BIGINT UNSIGNED NOT NULL,
    week         INT UNSIGNED    NOT NULL,
    home_team_id BIGINT UNSIGNED NOT NULL,
    away_team_id BIGINT UNSIGNED NOT NULL,
    home_goals   INT UNSIGNED    NULL,
    away_goals   INT UNSIGNED    NULL,
    played_at    TIMESTAMP(3)    NULL,
    edited_at    TIMESTAMP(3)    NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_matches_league_week_home (league_id, week, home_team_id),
    UNIQUE KEY uq_matches_league_week_away (league_id, week, away_team_id),
    UNIQUE KEY uq_matches_league_pair      (league_id, home_team_id, away_team_id),
    KEY idx_matches_home_team (home_team_id),
    KEY idx_matches_away_team (away_team_id),
    CONSTRAINT chk_matches_different_teams CHECK (home_team_id <> away_team_id),
    CONSTRAINT fk_matches_league
        FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    CONSTRAINT fk_matches_home_team
        FOREIGN KEY (home_team_id) REFERENCES teams(id) ON DELETE CASCADE,
    CONSTRAINT fk_matches_away_team
        FOREIGN KEY (away_team_id) REFERENCES teams(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
