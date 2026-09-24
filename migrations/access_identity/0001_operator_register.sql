-- 操作者册（ADR-0100 决定二第三条）：运营操作者主体（发行方 + sub）绑定唯一租户；授予按能力面
-- 显式登记、可撤销、带生效区间。册的列由产品的授权模型定死；行是租户取值（参数登记册 PAR-INT-08），
-- 本迁移不种任何行。
--
-- 三张表都只增不改：撤销是另一张表里的一行，不 UPDATE 授予。被撤的那笔授予是那段时间里他确实
-- 有权的证据，改掉它就答不出「某一刻谁能写哪册」。
--
-- 库里只登标识：不登凭据本体（ADR-0100 决定二第二条），不登姓名与联系方式（AGENTS 红线「敏感实例
-- 外置」）。

CREATE TABLE access_identity.operator (
    issuer      text        NOT NULL,
    subject     text        NOT NULL,
    tenant_id   text        NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 键不含租户：同一个主体至多一行，「一个主体绑两个租户」在库里无处可放。不存在跨租户主体
    -- （ADR-0100 决定二第三条，承 ADR-0021）靠的是这把键，不靠登记口记得先查。
    CONSTRAINT operator_pkey PRIMARY KEY (issuer, subject),

    -- 授予经这三列回指绑定：授予行的租户必须就是主体所绑的那个，跨租户授予过不了外键。
    CONSTRAINT operator_tenant_binding_key UNIQUE (issuer, subject, tenant_id),

    CONSTRAINT operator_not_blank
        CHECK (
            btrim(issuer) <> ''
            AND btrim(subject) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(basis_ref) <> ''
        )
);

CREATE TABLE access_identity.operator_grant (
    tenant_id           text        NOT NULL,
    grant_id            text        NOT NULL,
    issuer              text        NOT NULL,
    subject             text        NOT NULL,
    capability_face     text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    basis_ref           text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    -- 授予有自己的标识，不以（主体、能力面）为键：撤了再授是两笔授予，前一笔要原样留册。
    CONSTRAINT operator_grant_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT operator_grant_binding_fkey
        FOREIGN KEY (issuer, subject, tenant_id)
        REFERENCES access_identity.operator (issuer, subject, tenant_id),

    CONSTRAINT operator_grant_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(grant_id) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 只收已裁的两格。治理登记那一格只预留（授予模型按 ADR-0085 决定四另裁），不在这里；裁决落地
    -- 时由新迁移放开本条，与 accessidentity.CapabilityFace 同笔改。
    CONSTRAINT operator_grant_capability_face_decided
        CHECK (capability_face IN ('REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ')),

    -- 含起点、不含终点，终点缺席即不设终点。自然到期不存成状态，由区间对时点导出。
    CONSTRAINT operator_grant_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 铸造信封时那一查以主体发问、要取名下全部授予（ADR-0100 决定三），不以授予标识发问。
CREATE INDEX operator_grant_by_subject
    ON access_identity.operator_grant (issuer, subject);

CREATE TABLE access_identity.operator_grant_revocation (
    tenant_id   text        NOT NULL,
    grant_id    text        NOT NULL,
    revoked_at  timestamptz NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 一笔授予至多撤一次：撤销是终局，撤了再授是另一笔授予。
    CONSTRAINT operator_grant_revocation_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT operator_grant_revocation_grant_fkey
        FOREIGN KEY (tenant_id, grant_id)
        REFERENCES access_identity.operator_grant (tenant_id, grant_id),

    -- 时刻与依据缺一不成立；时刻已由 NOT NULL 守，这里守依据不是空白。
    CONSTRAINT operator_grant_revocation_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);
