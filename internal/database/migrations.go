package database

const schemaV1 = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organizations (
    id uuid PRIMARY KEY,
    parent_id uuid REFERENCES organizations(id),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    username text NOT NULL,
    display_name text NOT NULL,
    email text,
    password_hash text,
    oidc_subject text UNIQUE,
    organization_id uuid REFERENCES organizations(id),
    title text,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    active boolean NOT NULL DEFAULT true,
    must_change_password boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users(lower(username));

CREATE TABLE IF NOT EXISTS roles (
    id text PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions text[] NOT NULL DEFAULT '{}',
    system boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id text NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    scope jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY(user_id, role_id)
);

INSERT INTO roles(id,name,description,permissions,system) VALUES
('system_admin','System Admin','Full system configuration and security administration',ARRAY['*'],true),
('strategy_admin','Strategy Admin','Strategy structure and KPI administration',ARRAY['strategy:*','kpi:*','initiative:*','import:*','dashboard:read'],true),
('planning_admin','Planning Admin','Plan, template and workflow administration',ARRAY['plan:*','workflow:*','budget:*','scenario:*','decision:*','import:*','dashboard:read'],true),
('finance_manager','Finance Manager','Budget and financial performance',ARRAY['budget:*','forecast:*','dashboard:read'],true),
('planning_manager','Planning Manager','Enterprise planning operations',ARRAY['plan:*','strategy:read','kpi:read','project:read','scenario:*','decision:*','dashboard:read'],true),
('department_manager','Department Manager','Organization plan authoring, review and approval',ARRAY['plan:own','plan:organization','approval:*','kpi:organization','dashboard:read'],true),
('project_manager','Project Manager','Initiatives and projects',ARRAY['initiative:*','project:*','dashboard:read'],true),
('executive','Executive','Enterprise cockpit and decision information',ARRAY['dashboard:executive','dashboard:read','strategy:read','kpi:read','project:read','decision:read','intelligence:read','scenario:read','report:read','ai:query'],true),
('analyst','Analyst','Analysis and intelligence',ARRAY['dashboard:read','strategy:read','kpi:read','project:read','decision:read','intelligence:*','scenario:*','ai:query'],true),
('general_user','General User','Authorized read and input',ARRAY['dashboard:personal','strategy:read','kpi:read','plan:own','action:own'],true),
('auditor','Auditor','Audit and history read access',ARRAY['audit:read','history:read'],true)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    csrf_token text NOT NULL,
    auth_method text NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS system_settings (
    category text NOT NULL,
    key text NOT NULL,
    value jsonb,
    encrypted_value text,
    sensitive boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 1,
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(category,key),
    CHECK ((sensitive AND encrypted_value IS NOT NULL AND value IS NULL) OR (NOT sensitive AND value IS NOT NULL AND encrypted_value IS NULL))
);

CREATE TABLE IF NOT EXISTS strategies (
    id uuid PRIMARY KEY,
    parent_id uuid REFERENCES strategies(id),
    name text NOT NULL,
    kind text NOT NULL CHECK(kind IN ('vision','mission','theme','objective')),
    description text NOT NULL DEFAULT '',
    period_start date,
    period_end date,
    version integer NOT NULL DEFAULT 1,
    status text NOT NULL DEFAULT 'draft',
    classification text NOT NULL DEFAULT 'internal',
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS strategies_parent_idx ON strategies(parent_id);

CREATE TABLE IF NOT EXISTS kpis (
    id uuid PRIMARY KEY,
    strategy_id uuid REFERENCES strategies(id),
    parent_id uuid REFERENCES kpis(id),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    formula text NOT NULL DEFAULT '',
    unit text NOT NULL DEFAULT '',
    frequency text NOT NULL DEFAULT 'monthly',
    target numeric NOT NULL DEFAULT 0,
    actual numeric NOT NULL DEFAULT 0,
    warning_threshold numeric,
    critical_threshold numeric,
    weight numeric NOT NULL DEFAULT 0,
    source text NOT NULL DEFAULT '',
    classification text NOT NULL DEFAULT 'internal',
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY,
    strategy_id uuid REFERENCES strategies(id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'planned',
    progress numeric NOT NULL DEFAULT 0 CHECK(progress >= 0 AND progress <= 100),
    risk text NOT NULL DEFAULT 'low',
    budget numeric NOT NULL DEFAULT 0,
    actual_cost numeric NOT NULL DEFAULT 0,
    start_date date,
    end_date date,
    score jsonb NOT NULL DEFAULT '{}'::jsonb,
    classification text NOT NULL DEFAULT 'internal',
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans (
    id uuid PRIMARY KEY,
    title text NOT NULL,
    period text NOT NULL,
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    status text NOT NULL DEFAULT 'draft',
    version integer NOT NULL DEFAULT 1,
    content jsonb NOT NULL DEFAULT '{}'::jsonb,
    classification text NOT NULL DEFAULT 'internal',
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS workflow_definitions (
    id uuid PRIMARY KEY,
    resource_type text NOT NULL UNIQUE,
    enabled boolean NOT NULL DEFAULT false,
    steps jsonb NOT NULL DEFAULT '[]'::jsonb,
    rejection_policy text NOT NULL DEFAULT 'return_to_author',
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS workflow_instances (
    id uuid PRIMARY KEY,
    definition_id uuid NOT NULL REFERENCES workflow_definitions(id),
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    current_step integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'pending',
    history jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS decisions (
    id uuid PRIMARY KEY,
    title text NOT NULL,
    decision_date date,
    decision_maker_id uuid REFERENCES users(id),
    background text NOT NULL DEFAULT '',
    options jsonb NOT NULL DEFAULT '[]'::jsonb,
    decision text NOT NULL DEFAULT '',
    reason text NOT NULL DEFAULT '',
    evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
    related_kpi_id uuid REFERENCES kpis(id),
    related_project_id uuid REFERENCES projects(id),
    classification text NOT NULL DEFAULT 'internal',
    organization_id uuid REFERENCES organizations(id),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS intelligence_items (
    id uuid PRIMARY KEY,
    category text NOT NULL CHECK(category IN ('competitor','market','customer','regulation','technology','economic','investment','product')),
    title text NOT NULL,
    source_name text NOT NULL DEFAULT '',
    source_url text NOT NULL DEFAULT '',
    published_at timestamptz,
    raw_content text NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    importance integer NOT NULL DEFAULT 0 CHECK(importance BETWEEN 0 AND 100),
    company_relevance text NOT NULL DEFAULT '',
    potential_impact text NOT NULL DEFAULT '',
    risk text NOT NULL DEFAULT '',
    opportunity text NOT NULL DEFAULT '',
    recommended_action text NOT NULL DEFAULT '',
    classification text NOT NULL DEFAULT 'internal',
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS intelligence_category_idx ON intelligence_items(category,published_at DESC);
CREATE INDEX IF NOT EXISTS intelligence_search_idx ON intelligence_items USING gin(to_tsvector('simple',title || ' ' || summary || ' ' || raw_content));

CREATE TABLE IF NOT EXISTS scenarios (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','simulation','review','approved','applied')),
    assumptions jsonb NOT NULL DEFAULT '{}'::jsonb,
    results jsonb,
    strategy_id uuid REFERENCES strategies(id),
    classification text NOT NULL DEFAULT 'confidential',
    organization_id uuid REFERENCES organizations(id),
    owner_id uuid REFERENCES users(id),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_interactions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    use_case text NOT NULL,
    model_name text NOT NULL,
    prompt_encrypted text NOT NULL,
    response_encrypted text NOT NULL,
    evidence jsonb NOT NULL DEFAULT '[]'::jsonb,
    confidence numeric,
    outcome text NOT NULL DEFAULT 'success',
    prompt_tokens bigint NOT NULL DEFAULT 0,
    response_tokens bigint NOT NULL DEFAULT 0,
    latency_ms bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION planexus_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'append-only record is immutable';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS ai_interactions_no_update ON ai_interactions;
CREATE TRIGGER ai_interactions_no_update BEFORE UPDATE OR DELETE ON ai_interactions
FOR EACH ROW EXECUTE FUNCTION planexus_immutable();

CREATE TABLE IF NOT EXISTS import_jobs (
    id uuid PRIMARY KEY,
    entity_type text NOT NULL CHECK(entity_type IN ('strategy','kpi','project')),
    file_name text NOT NULL,
    mapping jsonb NOT NULL DEFAULT '{}'::jsonb,
    preview_data jsonb NOT NULL DEFAULT '[]'::jsonb,
    validation_errors jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL DEFAULT 'preview' CHECK(status IN ('preview','imported','rolled_back','failed')),
    total_rows integer NOT NULL DEFAULT 0,
    valid_rows integer NOT NULL DEFAULT 0,
    invalid_rows integer NOT NULL DEFAULT 0,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    imported_at timestamptz,
    rolled_back_at timestamptz
);
CREATE TABLE IF NOT EXISTS import_job_records (
    import_job_id uuid NOT NULL REFERENCES import_jobs(id) ON DELETE RESTRICT,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY(import_job_id,resource_type,resource_id)
);

CREATE TABLE IF NOT EXISTS notification_rules (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    entity_type text NOT NULL CHECK(entity_type IN ('kpi','project','intelligence','key')),
    condition_field text NOT NULL,
    operator text NOT NULL CHECK(operator IN ('lt','lte','eq','gte','gt')),
    threshold text NOT NULL,
    severity text NOT NULL DEFAULT 'warning' CHECK(severity IN ('info','warning','critical')),
    channels text[] NOT NULL DEFAULT ARRAY['system']::text[],
    global boolean NOT NULL DEFAULT false,
    enabled boolean NOT NULL DEFAULT true,
    cooldown_minutes integer NOT NULL DEFAULT 1440 CHECK(cooldown_minutes BETWEEN 1 AND 43200),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS notifications (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id uuid REFERENCES notification_rules(id) ON DELETE SET NULL,
    dedupe_key text NOT NULL UNIQUE,
    severity text NOT NULL,
    title text NOT NULL,
    message text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    channels text[] NOT NULL DEFAULT ARRAY['system']::text[],
    delivery_status jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications(user_id,read_at,created_at DESC);

CREATE TABLE IF NOT EXISTS backup_jobs (
    id uuid PRIMARY KEY,
    operation text NOT NULL CHECK(operation IN ('export','validate','restore')),
    file_name text NOT NULL DEFAULT '',
    status text NOT NULL CHECK(status IN ('running','completed','failed')),
    size_bytes bigint NOT NULL DEFAULT 0,
    checksum text NOT NULL DEFAULT '',
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE TABLE IF NOT EXISTS personal_access_keys (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    prefix text NOT NULL,
    secret_hash bytea NOT NULL UNIQUE,
    scopes text[] NOT NULL DEFAULT '{}',
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    replaced_by uuid REFERENCES personal_access_keys(id),
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id uuid PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_id uuid REFERENCES users(id),
    actor_name text NOT NULL DEFAULT 'system',
    event_type text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    action text NOT NULL,
    outcome text NOT NULL DEFAULT 'success',
    ip_address inet,
    user_agent text,
    details jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS audit_logs_occurred_idx ON audit_logs(occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_resource_idx ON audit_logs(resource_type,resource_id);

CREATE OR REPLACE FUNCTION planexus_audit_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit logs are immutable';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS audit_logs_no_update ON audit_logs;
CREATE TRIGGER audit_logs_no_update BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION planexus_audit_immutable();

INSERT INTO workflow_definitions(id,resource_type,enabled,steps)
VALUES ('00000000-0000-0000-0000-000000000001','plan',false,'[]'::jsonb)
ON CONFLICT(resource_type) DO NOTHING;
`
