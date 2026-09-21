-- Seed two sample timeseries for the root user on database init.
-- Creates a "Sample Data" folder under root's home (id=3) containing
-- two Timeseries nodes. Sample data points are seeded separately in the
-- QuestDB migration (migrations/questdb/000002_seed_data.sql).

-- Sample Data folder under root's home node (id=3).
INSERT INTO node (id, display_name, parent_id) VALUES (4, 'Sample Data', 3);
SELECT setval('node_id_seq', 4);

-- Sample timeseries 1: Air Temperature.
INSERT INTO node (id, display_name, kind, parent_id, metadata)
VALUES (5, 'Air Temperature', 'Timeseries', 4,
        '{"unit": "°C", "lat": 49.9427, "lon": 11.5761, "resolution": 3600}');
SELECT setval('node_id_seq', 5);

-- Sample timeseries 2: Soil Moisture.
INSERT INTO node (id, display_name, kind, parent_id, metadata)
VALUES (6, 'Soil Moisture', 'Timeseries', 4,
        '{"unit": "%", "lat": 49.9427, "lon": 11.5761, "resolution": 3600}');
SELECT setval('node_id_seq', 6);
