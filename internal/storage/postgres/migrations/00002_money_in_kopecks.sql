-- +goose Up
ALTER TABLE users
    ALTER COLUMN balance   TYPE BIGINT USING (balance * 100)::BIGINT,
    ALTER COLUMN withdrawn TYPE BIGINT USING (withdrawn * 100)::BIGINT;

ALTER TABLE orders
    ALTER COLUMN accrual TYPE BIGINT USING (accrual * 100)::BIGINT;

ALTER TABLE withdrawals
    ALTER COLUMN sum TYPE BIGINT USING (sum * 100)::BIGINT;

COMMENT ON COLUMN users.balance   IS 'bonus points in kopecks (1/100 of a point)';
COMMENT ON COLUMN users.withdrawn IS 'bonus points in kopecks (1/100 of a point)';
COMMENT ON COLUMN orders.accrual  IS 'bonus points in kopecks (1/100 of a point)';
COMMENT ON COLUMN withdrawals.sum IS 'bonus points in kopecks (1/100 of a point)';

-- +goose Down
ALTER TABLE withdrawals
    ALTER COLUMN sum TYPE NUMERIC(14,2) USING sum / 100.0;

ALTER TABLE orders
    ALTER COLUMN accrual TYPE NUMERIC(14,2) USING accrual / 100.0;

ALTER TABLE users
    ALTER COLUMN balance   TYPE NUMERIC(14,2) USING balance / 100.0,
    ALTER COLUMN withdrawn TYPE NUMERIC(14,2) USING withdrawn / 100.0;

COMMENT ON COLUMN users.balance   IS NULL;
COMMENT ON COLUMN users.withdrawn IS NULL;
COMMENT ON COLUMN orders.accrual  IS NULL;
COMMENT ON COLUMN withdrawals.sum IS NULL;
