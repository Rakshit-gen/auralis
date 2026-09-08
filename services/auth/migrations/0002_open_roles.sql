-- Every account gets USER, CREATOR, and ADMIN. The platform no longer gates
-- authoring, AI generation, or review behind a separate role: anyone who signs
-- up can create and manage shows and episodes. The role column and the
-- middleware stay in place so service-to-service checks keep working and a
-- future deployment can tighten this again.

ALTER TABLE users ALTER COLUMN roles SET DEFAULT ARRAY['USER', 'CREATOR', 'ADMIN'];

UPDATE users
SET roles = ARRAY['USER', 'CREATOR', 'ADMIN'],
    updated_at = now()
WHERE NOT (roles @> ARRAY['USER', 'CREATOR', 'ADMIN']);
