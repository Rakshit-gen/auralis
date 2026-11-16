-- 0002 handed ADMIN to every account and made it the column default. Signup
-- stopped granting it, but the backfilled accounts and the default stayed.
-- Take ADMIN back from everyone; BootstrapAdmin re-promotes the configured
-- admin on the next start. CREATOR from the backfill is left alone.

ALTER TABLE users ALTER COLUMN roles SET DEFAULT ARRAY['USER'];

UPDATE users
SET roles = array_remove(roles, 'ADMIN'),
    updated_at = now()
WHERE 'ADMIN' = ANY(roles);
