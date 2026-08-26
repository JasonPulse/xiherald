-- Grant the Herald read-only access to exactly the tables it reads.
--
-- Run this against the game database as root. Nothing here is idempotent-
-- unsafe: re-running it is harmless.
--
-- Set the password to match the Kubernetes secret before running.

CREATE USER IF NOT EXISTS 'xiherald'@'%' IDENTIFIED BY 'CHANGE_ME';

-- Character identity, progression and location.
GRANT SELECT ON xidb.chars            TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_stats       TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_look        TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_jobs        TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_exp         TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_job_points  TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_skills      TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_profile     TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_points      TO 'xiherald'@'%';
GRANT SELECT ON xidb.char_history     TO 'xiherald'@'%';

-- Reference tables for skill caps and zone names.
GRANT SELECT ON xidb.skill_caps       TO 'xiherald'@'%';
GRANT SELECT ON xidb.zone_settings    TO 'xiherald'@'%';

-- Who is logged in. One row per online character.
GRANT SELECT ON xidb.accounts_sessions TO 'xiherald'@'%';

-- The Herald reads gil, which lives in the inventory at location 0 slot 0.
-- A column grant keeps the rest of every character's inventory out of reach.
GRANT SELECT (charid, location, slot, quantity) ON xidb.char_inventory TO 'xiherald'@'%';

FLUSH PRIVILEGES;

-- Confirm the result:
--   SHOW GRANTS FOR 'xiherald'@'%';
