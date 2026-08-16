DROP TRIGGER IF EXISTS events_no_delete ON events;
DROP TRIGGER IF EXISTS events_no_update ON events;
DROP FUNCTION IF EXISTS forbid_event_mutation();
DROP TABLE IF EXISTS events;
