CREATE TABLE idempotency_keys (
    idempotency_key VARCHAR(120) NOT NULL,
    request_hash    CHAR(64)     NOT NULL,
    state           ENUM('in_progress', 'completed') NOT NULL,
    response_status INT UNSIGNED NULL,
    response_body   LONGTEXT     NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (idempotency_key),
    CONSTRAINT chk_idempotency_completion
        CHECK (
            (state = 'in_progress' AND response_status IS NULL AND response_body IS NULL)
            OR
            (state = 'completed' AND response_status IS NOT NULL AND response_body IS NOT NULL)
        )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
