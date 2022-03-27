BEGIN;

DROP TABLE IF EXISTS clearchat;
DROP TABLE IF EXISTS broadcaster;

DROP INDEX IF EXISTS idx_clearchat_type;
DROP INDEX IF EXISTS idx_clearchat_channel_at;
DROP INDEX IF EXISTS idx_clearchat_username_at;

DROP TYPE IF EXISTS langISO31661;
DROP TYPE IF EXISTS ban_type;

COMMIT;
