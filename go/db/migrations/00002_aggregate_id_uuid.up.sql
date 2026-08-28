-- Every aggregate id in this system is a UUID by convention; this makes it a
-- constraint the database enforces too, not just the Twirp handlers that
-- validate it on the way in.
ALTER TABLE events ALTER COLUMN aggregate_id TYPE uuid USING aggregate_id::uuid;
