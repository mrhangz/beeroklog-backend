ALTER TABLE users
    ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_users_is_admin ON users (is_admin) WHERE is_admin = true;
