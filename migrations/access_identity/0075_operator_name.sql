-- 操作者的姓名（2026-11-27 用户定，AGENTS.md 红线「敏感实例外置」给操作者姓名开的那一条例外）。
-- 操作者册只登标识（发行方 + sub）：sub 是发行方给的一串号，人认不出是谁。派工、放波次这些地方要让人看得出「这是谁」，
-- 所以另立这一张表登姓名。只登姓名这一格——不登联系方式，不登凭据本体，那两样仍然不进库。
--
-- 这张表可以改、可以删，不照操作者册「只增不改」办：姓名不是授权的证据，人改了名、写错了、离开了，
-- 都要能改掉或删掉；留一串旧姓名的历史才是多存了不该存的。没登姓名的操作者没有行，界面上照旧示他的 sub。

CREATE TABLE access_identity.operator_name (
    issuer      text        NOT NULL,
    subject     text        NOT NULL,
    tenant_id   text        NOT NULL,
    name        text        NOT NULL,
    recorded_by text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT operator_name_pkey PRIMARY KEY (issuer, subject),
    -- 经这三列回指绑定：姓名行的租户就是主体所绑的那个。
    CONSTRAINT operator_name_operator_fkey
        FOREIGN KEY (issuer, subject, tenant_id)
        REFERENCES access_identity.operator (issuer, subject, tenant_id),
    CONSTRAINT operator_name_not_blank CHECK (btrim(name) <> '' AND btrim(recorded_by) <> ''),
    CONSTRAINT operator_name_length CHECK (char_length(name) <= 60)
);

CREATE INDEX operator_name_tenant_idx ON access_identity.operator_name (tenant_id);
