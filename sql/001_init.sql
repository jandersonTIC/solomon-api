CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    apple_sub TEXT UNIQUE NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS accounts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    balance_cents BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'BRL',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts(user_id);

CREATE TABLE IF NOT EXISTS transactions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    account_id BIGINT REFERENCES accounts(id),
    type SMALLINT NOT NULL,
    status SMALLINT NOT NULL DEFAULT 0,
    date DATE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'BRL',
    recurrence_type SMALLINT NOT NULL DEFAULT 0,
    installment_cur SMALLINT,
    installment_tot SMALLINT,
    year_month INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tx_user_ym ON transactions(user_id, year_month);
CREATE INDEX IF NOT EXISTS idx_tx_user_ym_type ON transactions(user_id, year_month, type);
CREATE INDEX IF NOT EXISTS idx_tx_user_desc ON transactions(user_id, description);

CREATE TABLE IF NOT EXISTS month_balances (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    year_month INT NOT NULL,
    carry_over_cents BIGINT NOT NULL DEFAULT 0,
    UNIQUE(user_id, year_month)
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
