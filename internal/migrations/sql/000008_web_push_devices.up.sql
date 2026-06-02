CREATE TABLE IF NOT EXISTS web_push_devices (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES app_sessions(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    enabled_categories JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_registered_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_web_push_devices_session ON web_push_devices(session_id);
CREATE INDEX IF NOT EXISTS idx_web_push_devices_endpoint ON web_push_devices(endpoint);
