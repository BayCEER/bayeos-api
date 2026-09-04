DROP INDEX IF EXISTS idx_node_metadata;

ALTER TABLE node DROP COLUMN IF EXISTS metadata;
