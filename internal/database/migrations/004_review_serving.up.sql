ALTER TABLE reviews
    ADD COLUMN serving_size_ml INTEGER,
    ADD COLUMN serving_count INTEGER;

COMMENT ON COLUMN reviews.serving_size_ml IS 'Optional ml per unit (e.g. 330, 500). Null = not logged.';
COMMENT ON COLUMN reviews.serving_count IS 'Optional number of units. Null = not logged.';
