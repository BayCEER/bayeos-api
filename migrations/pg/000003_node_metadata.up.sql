ALTER TABLE node ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}';

CREATE INDEX idx_node_metadata ON node USING gin (metadata);
