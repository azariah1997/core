INSERT INTO applications(slug,name) VALUES ('demo','Demo App') ON CONFLICT(slug) DO NOTHING;
INSERT INTO users(identity_subject,display_name,locale,timezone)
VALUES ('local-demo-user','Local Developer','en-GB','Europe/London')
ON CONFLICT(identity_subject) DO NOTHING;
