CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS user_actor_identity (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL UNIQUE,
    main_key_id TEXT NOT NULL UNIQUE,
    private_key_multibase TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS as_person (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS as_application (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS as_ordered_collection (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS as_note (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS as_update (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_team_membership (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_team (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_ticket_tracker (
    id BIGSERIAL UNIQUE,
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    internal_id UUID UNIQUE,
    slug TEXT UNIQUE,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_local BOOLEAN NOT NULL DEFAULT FALSE
);

ALTER TABLE ff_ticket_tracker
    DROP COLUMN IF EXISTS name;

ALTER TABLE ff_ticket_tracker
    DROP COLUMN IF EXISTS summary;

CREATE TABLE IF NOT EXISTS ff_repository (
    id BIGSERIAL UNIQUE,
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    internal_id UUID UNIQUE,
    slug TEXT UNIQUE,
    filesystem_path TEXT UNIQUE,
    ticket_tracker_internal_id UUID REFERENCES ff_ticket_tracker(internal_id) ON DELETE SET NULL,
    is_local BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS ff_repository_ticket_tracker_internal_id_idx
    ON ff_repository(ticket_tracker_internal_id);

CREATE TABLE IF NOT EXISTS ff_repository_member (
    id BIGSERIAL PRIMARY KEY,
    repository_internal_id UUID NOT NULL REFERENCES ff_repository(internal_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(repository_internal_id, user_id)
);

CREATE INDEX IF NOT EXISTS ff_repository_member_repository_internal_id_idx
    ON ff_repository_member(repository_internal_id);

CREATE INDEX IF NOT EXISTS ff_repository_member_user_id_idx
    ON ff_repository_member(user_id);

CREATE TABLE IF NOT EXISTS ff_patch_tracker (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_ticket (
    id BIGSERIAL UNIQUE,
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    internal_id UUID UNIQUE,
    slug TEXT UNIQUE,
    tracker_internal_id UUID REFERENCES ff_ticket_tracker(internal_id) ON DELETE CASCADE,
    repository_internal_id UUID REFERENCES ff_repository(internal_id) ON DELETE CASCADE,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    is_local BOOLEAN NOT NULL DEFAULT FALSE
);

ALTER TABLE ff_ticket
    ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS ff_ticket_tracker_internal_id_idx
    ON ff_ticket(tracker_internal_id);

CREATE INDEX IF NOT EXISTS ff_ticket_repository_internal_id_idx
    ON ff_ticket(repository_internal_id);

CREATE INDEX IF NOT EXISTS ff_ticket_created_priority_idx
    ON ff_ticket(created_at ASC, priority DESC, id ASC);

CREATE TABLE IF NOT EXISTS ff_ticket_comment (
    id BIGSERIAL UNIQUE,
    internal_id UUID UNIQUE,
    slug TEXT UNIQUE,
    note_primary_key TEXT PRIMARY KEY REFERENCES as_note(primary_key) ON DELETE CASCADE,
    ticket_internal_id UUID NOT NULL REFERENCES ff_ticket(internal_id) ON DELETE CASCADE,
    repository_internal_id UUID NOT NULL REFERENCES ff_repository(internal_id) ON DELETE CASCADE,
    in_reply_to_note_primary_key TEXT REFERENCES as_note(primary_key) ON DELETE SET NULL,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    recipient_actor_id TEXT NOT NULL,
    is_local BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ff_ticket_comment_ticket_internal_id_idx
    ON ff_ticket_comment(ticket_internal_id);

CREATE INDEX IF NOT EXISTS ff_ticket_comment_repository_internal_id_idx
    ON ff_ticket_comment(repository_internal_id);

CREATE INDEX IF NOT EXISTS ff_ticket_comment_in_reply_to_note_primary_key_idx
    ON ff_ticket_comment(in_reply_to_note_primary_key);

CREATE TABLE IF NOT EXISTS ff_ticket_assignee (
    id BIGSERIAL PRIMARY KEY,
    ticket_internal_id UUID NOT NULL REFERENCES ff_ticket(internal_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(ticket_internal_id, user_id)
);

CREATE INDEX IF NOT EXISTS ff_ticket_assignee_ticket_internal_id_idx
    ON ff_ticket_assignee(ticket_internal_id);

CREATE INDEX IF NOT EXISTS ff_ticket_assignee_user_id_idx
    ON ff_ticket_assignee(user_id);

CREATE INDEX IF NOT EXISTS ff_ticket_assignee_user_ticket_idx
    ON ff_ticket_assignee(user_id, ticket_internal_id);

CREATE TABLE IF NOT EXISTS notification (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    ticket_internal_id UUID NOT NULL REFERENCES ff_ticket(internal_id) ON DELETE CASCADE,
    repository_internal_id UUID NOT NULL REFERENCES ff_repository(internal_id) ON DELETE CASCADE,
    assigned_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    unread BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notification_user_id_created_at_idx
    ON notification(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS notification_user_id_unread_idx
    ON notification(user_id, unread);

CREATE INDEX IF NOT EXISTS notification_ticket_internal_id_idx
    ON notification(ticket_internal_id);

CREATE TABLE IF NOT EXISTS ff_patch (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_commit (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_branch (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_factory (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_enum (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_enum_value (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_field (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_milestone (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_release (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_review (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_review_thread (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_code_quote (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_suggestion (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_approval (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_organization_membership (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_ssh_public_key (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_ticket_dependency (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_edit (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_grant (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_revoke (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_assign (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_apply (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_resolve (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ff_push (
    primary_key TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ap_object_unknown (
    primary_key TEXT PRIMARY KEY,
    ap_type TEXT NOT NULL,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ap_object_version (
    id BIGSERIAL PRIMARY KEY,
    object_primary_key TEXT NOT NULL,
    object_type TEXT NOT NULL,
    body JSONB NOT NULL,
    source_update_primary_key TEXT,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ap_object_version_object_primary_key_created_at_idx
    ON ap_object_version(object_primary_key, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS oauth_client (
    client_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    redirect_uris JSONB NOT NULL,
    allowed_scopes JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS oauth_client_secret (
    client_id TEXT PRIMARY KEY REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    secret_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS oauth_consent_grant (
    grant_id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_scopes JSONB NOT NULL,
    granted_scopes JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oauth_authorization_code (
    signature TEXT PRIMARY KEY,
    request_id TEXT UNIQUE NOT NULL,
    client_id TEXT NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    grant_id TEXT REFERENCES oauth_consent_grant(grant_id) ON DELETE SET NULL,
    request_json JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oauth_pkce_request (
    signature TEXT PRIMARY KEY,
    request_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS oauth_access_token (
    signature TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    client_id TEXT NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    grant_id TEXT REFERENCES oauth_consent_grant(grant_id) ON DELETE SET NULL,
    request_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oauth_refresh_token (
    signature TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    access_token_signature TEXT,
    family_id TEXT NOT NULL,
    parent_signature TEXT,
    client_id TEXT NOT NULL REFERENCES oauth_client(client_id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    grant_id TEXT REFERENCES oauth_consent_grant(grant_id) ON DELETE SET NULL,
    request_json JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oauth_client_assertion_jti (
    jti TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS oauth_consent_grant_client_id_idx
    ON oauth_consent_grant(client_id);

CREATE INDEX IF NOT EXISTS oauth_consent_grant_user_id_idx
    ON oauth_consent_grant(user_id);

CREATE INDEX IF NOT EXISTS oauth_authorization_code_client_id_idx
    ON oauth_authorization_code(client_id);

CREATE INDEX IF NOT EXISTS oauth_authorization_code_user_id_idx
    ON oauth_authorization_code(user_id);

CREATE INDEX IF NOT EXISTS oauth_authorization_code_expires_at_idx
    ON oauth_authorization_code(expires_at);

CREATE INDEX IF NOT EXISTS oauth_access_token_request_id_idx
    ON oauth_access_token(request_id);

CREATE INDEX IF NOT EXISTS oauth_access_token_client_id_idx
    ON oauth_access_token(client_id);

CREATE INDEX IF NOT EXISTS oauth_access_token_user_id_idx
    ON oauth_access_token(user_id);

CREATE INDEX IF NOT EXISTS oauth_access_token_grant_id_idx
    ON oauth_access_token(grant_id);

CREATE INDEX IF NOT EXISTS oauth_access_token_expires_at_idx
    ON oauth_access_token(expires_at);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_request_id_idx
    ON oauth_refresh_token(request_id);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_client_id_idx
    ON oauth_refresh_token(client_id);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_user_id_idx
    ON oauth_refresh_token(user_id);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_grant_id_idx
    ON oauth_refresh_token(grant_id);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_family_id_idx
    ON oauth_refresh_token(family_id);

CREATE INDEX IF NOT EXISTS oauth_refresh_token_expires_at_idx
    ON oauth_refresh_token(expires_at);

CREATE INDEX IF NOT EXISTS oauth_client_assertion_jti_expires_at_idx
    ON oauth_client_assertion_jti(expires_at);
