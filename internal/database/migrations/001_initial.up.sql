CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    avatar_url    TEXT NOT NULL DEFAULT '',
    auth_provider TEXT NOT NULL,
    auth_provider_id TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (auth_provider, auth_provider_id)
);

CREATE TABLE beers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    brewery    TEXT NOT NULL DEFAULT '',
    style      TEXT NOT NULL DEFAULT '',
    abv        DOUBLE PRECISION,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_beers_name_brewery ON beers (lower(name), lower(brewery));

CREATE TABLE reviews (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    beer_id     UUID NOT NULL REFERENCES beers(id) ON DELETE CASCADE,
    rating      DOUBLE PRECISION NOT NULL DEFAULT 0,
    review_text TEXT NOT NULL DEFAULT '',
    tasted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reviews_user ON reviews (user_id, created_at DESC);
CREATE INDEX idx_reviews_beer ON reviews (beer_id, created_at DESC);
CREATE INDEX idx_reviews_created ON reviews (created_at DESC);

CREATE TABLE review_photos (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id   UUID NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_review_photos_review ON review_photos (review_id, sort_order);
