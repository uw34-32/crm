-- One family per login. Rotation adds tokens to the family; reuse or logout
-- revokes the family as a whole. Locking the family row serializes rotations.
CREATE TABLE refresh_token_families (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX refresh_token_families_user_id_idx ON refresh_token_families (user_id);

-- Only the SHA-256 of a token is stored. rotated_at marks a token as spent; it
-- stays in place until purged so that presenting it again is detected as reuse.
CREATE TABLE refresh_tokens (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    family_id  UUID NOT NULL REFERENCES refresh_token_families (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    rotated_at TIMESTAMPTZ
);

CREATE INDEX refresh_tokens_family_id_idx ON refresh_tokens (family_id);
