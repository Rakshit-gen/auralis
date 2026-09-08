-- One PostgreSQL instance, one database per service. Each service connects only
-- to its own database and never to another's, matching the deployed topology
-- where these are separate managed databases.
CREATE DATABASE auth;
CREATE DATABASE users;
CREATE DATABASE content;
CREATE DATABASE playback;
CREATE DATABASE analytics;
CREATE DATABASE ai_media;
CREATE DATABASE recommendation;
