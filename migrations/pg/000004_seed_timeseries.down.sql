-- Remove the seeded sample timeseries and their parent folder.
-- The folder row CASCADE-deletes its child nodes and their privileges.
DELETE FROM node WHERE id IN (5, 6, 7);
