-- +goose Up
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK (purpose IN ('invite', 'reset_password', 'confirm_email')),
    token_hash TEXT NOT NULL UNIQUE,
    role TEXT CHECK (role IN ('admin', 'user')),
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX auth_tokens_email_purpose_idx ON auth_tokens (email, purpose);
CREATE INDEX auth_tokens_expires_at_idx ON auth_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS settings;
