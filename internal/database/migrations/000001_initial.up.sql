CREATE TABLE users (
        telegram_id BIGINT PRIMARY KEY,
        user_name   VARCHAR(255),

        created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

        statuses      VARCHAR(50),

        subscription          VARCHAR(50),
        subscription_status   VARCHAR(50),
        subscription_end_at   TIMESTAMPTZ,
        subscription_start_at TIMESTAMPTZ,

        limits_photo       INT    NOT NULL DEFAULT 0,
        limits_chat       INT    NOT NULL DEFAULT 0,
        remaining_photo     INT    NOT NULL DEFAULT 0,
        remaining_chat     INT    NOT NULL DEFAULT 0,
        used_photo          INT    NOT NULL DEFAULT 0,
        used_chat          INT    NOT NULL DEFAULT 0,

        used_trial    INT    NOT NULL DEFAULT 0
);
