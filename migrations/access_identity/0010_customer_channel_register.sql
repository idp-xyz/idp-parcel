-- 客户渠道册（ADR-0139 决定二；票 first-party-api/00）：首方客户族的客户端主体（客户端凭据令牌的发行方 + sub）绑定唯一
-- （租户、客户账户），一次登定、不可改指；授予按客户面能力显式登记、可撤销、带生效区间。册的列由产品定；行是租户取值
-- （参数登记册 PAR-INT-13），本迁移不种任何行。
--
-- 行形照集成客户端册（0003），三张表只增不改，理由同 0001：撤销是另一张表里的一行，被撤的授予是那段时间里它确实有权的
-- 证据。库里只登标识与凭据引用：客户端密钥、私钥与证书本体都不落库。
--
-- 没有来源一列：首方族的来源就是这个客户端本身，由主体推出（accessidentity.CustomerClientSubject 的头注）。

CREATE TABLE access_identity.customer_client (
    issuer                           text        NOT NULL,
    subject                          text        NOT NULL,
    tenant_id                        text        NOT NULL,
    customer_account_id              text        NOT NULL,
    credential_ref                   text        NOT NULL,
    certificate_bound_token_required boolean     NOT NULL,
    basis_ref                        text        NOT NULL,
    recorded_at                      timestamptz NOT NULL DEFAULT now(),

    -- 键不含租户与客户账户：同一个客户端主体至多一行，「一个客户端代表两个客户账户」在库里无处可放。首发一客户端一客户
    -- 账户（ADR-0139 越权风险点「客户系统的令牌是否允许携带多个客户账户」的复核建议）；1:N 归票 first-party-api/01，届时
    -- 改的是这一条键与 accessidentity 的绑定形状。
    CONSTRAINT customer_client_pkey PRIMARY KEY (issuer, subject),

    -- 授予经这三列回指绑定：授予行的租户必须就是客户端所绑的那个。
    CONSTRAINT customer_client_tenant_binding_key UNIQUE (issuer, subject, tenant_id),

    CONSTRAINT customer_client_not_blank
        CHECK (
            btrim(issuer) <> ''
            AND btrim(subject) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(credential_ref) <> ''
            AND btrim(basis_ref) <> ''
        )
);

CREATE TABLE access_identity.customer_client_grant (
    tenant_id           text        NOT NULL,
    grant_id            text        NOT NULL,
    issuer              text        NOT NULL,
    subject             text        NOT NULL,
    capability          text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    basis_ref           text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_client_grant_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT customer_client_grant_binding_fkey
        FOREIGN KEY (issuer, subject, tenant_id)
        REFERENCES access_identity.customer_client (issuer, subject, tenant_id),

    CONSTRAINT customer_client_grant_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(grant_id) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 与 accessidentity.CustomerCapability 同一份封闭集，两处同笔改：客户面各口换上首方 Intake 时各加自己那一项。
    CONSTRAINT customer_client_grant_capability_decided
        CHECK (capability IN ('SHIPMENT_REQUEST_SUBMISSION')),

    -- 含起点、不含终点，终点缺席即不设终点。
    CONSTRAINT customer_client_grant_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 铸造信封时那一查以客户端主体发问、要取名下全部授予。
CREATE INDEX customer_client_grant_by_subject
    ON access_identity.customer_client_grant (issuer, subject);

CREATE TABLE access_identity.customer_client_grant_revocation (
    tenant_id   text        NOT NULL,
    grant_id    text        NOT NULL,
    revoked_at  timestamptz NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 一笔授予至多撤一次：撤销是终局，撤了再授是另一笔授予。
    CONSTRAINT customer_client_grant_revocation_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT customer_client_grant_revocation_grant_fkey
        FOREIGN KEY (tenant_id, grant_id)
        REFERENCES access_identity.customer_client_grant (tenant_id, grant_id),

    CONSTRAINT customer_client_grant_revocation_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);
