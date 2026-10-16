-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL     PRIMARY KEY,
    login         TEXT          NOT NULL UNIQUE,
    password_hash TEXT          NOT NULL,
    balance       NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    withdrawn     NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (withdrawn >= 0),
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
    number      TEXT          PRIMARY KEY,
    user_id     BIGINT        NOT NULL REFERENCES users (id),
    status      TEXT          NOT NULL DEFAULT 'NEW'
                              CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
    accrual     NUMERIC(14,2) CHECK (accrual >= 0),
    uploaded_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS orders_user_id_uploaded_at_idx ON orders (user_id, uploaded_at DESC);
CREATE INDEX IF NOT EXISTS orders_pending_idx ON orders (uploaded_at) WHERE status IN ('NEW', 'PROCESSING');

CREATE TABLE IF NOT EXISTS withdrawals (
    id           BIGSERIAL     PRIMARY KEY,
    user_id      BIGINT        NOT NULL REFERENCES users (id),
    order_number TEXT          NOT NULL,
    sum          NUMERIC(14,2) NOT NULL CHECK (sum > 0),
    processed_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS withdrawals_user_id_processed_at_idx ON withdrawals (user_id, processed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS users;
