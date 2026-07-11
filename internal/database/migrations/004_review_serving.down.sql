ALTER TABLE reviews
    DROP COLUMN IF EXISTS serving_count,
    DROP COLUMN IF EXISTS serving_size_ml;
