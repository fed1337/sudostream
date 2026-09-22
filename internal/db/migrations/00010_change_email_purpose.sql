-- +goose Up
ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS auth_tokens_purpose_check;
ALTER TABLE auth_tokens ADD CONSTRAINT auth_tokens_purpose_check
    CHECK (purpose IN (
        'invite',
        'reset_password',
        'confirm_email',
        'login_2fa',
        'change_email'
    ));

-- +goose Down
ALTER TABLE auth_tokens DROP CONSTRAINT IF EXISTS auth_tokens_purpose_check;
ALTER TABLE auth_tokens ADD CONSTRAINT auth_tokens_purpose_check
    CHECK (purpose IN (
        'invite',
        'reset_password',
        'confirm_email',
        'login_2fa'
    ));
