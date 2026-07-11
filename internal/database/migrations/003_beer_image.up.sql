ALTER TABLE beers
    ADD COLUMN image_storage_key TEXT;

COMMENT ON COLUMN beers.image_storage_key IS 'Optional S3 key for the beer pack shot / can / bottle image.';
