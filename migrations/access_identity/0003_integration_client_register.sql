-- 集成客户端册（ADR-0149 决定三；票 operator-channel/11）：外部系统的客户端主体（客户端凭据令牌的发行方 + sub）绑定唯一
-- 租户与一个来源身份；授予按事实类型显式登记、可撤销、带生效区间。册的列由产品定；行是租户取值（参数登记册
-- PAR-INT-09），本迁移不种任何行。
--
-- 三张表只增不改，理由同 0001：撤销是另一张表里的一行，被撤的授予是那段时间里它确实有权的证据。
--
-- 库里只登标识与凭据引用：客户端密钥、私钥与证书本体都不落库（ADR-0149 越权风险点 4），凭据引用只指向租户约定的
-- 受限存储里那一份的出处。

CREATE TABLE access_identity.integration_client (
    issuer                           text        NOT NULL,
    subject                          text        NOT NULL,
    tenant_id                        text        NOT NULL,
    source_ref                       text        NOT NULL,
    credential_ref                   text        NOT NULL,
    certificate_bound_token_required boolean     NOT NULL,
    basis_ref                        text        NOT NULL,
    recorded_at                      timestamptz NOT NULL DEFAULT now(),

    -- 键不含租户：同一个客户端主体至多一行，「一个客户端绑两个租户」在库里无处可放，理由同 0001 的操作者键。
    CONSTRAINT integration_client_pkey PRIMARY KEY (issuer, subject),

    -- 授予经这三列回指绑定：授予行的租户必须就是客户端所绑的那个。
    CONSTRAINT integration_client_tenant_binding_key UNIQUE (issuer, subject, tenant_id),

    CONSTRAINT integration_client_not_blank
        CHECK (
            btrim(issuer) <> ''
            AND btrim(subject) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(credential_ref) <> ''
            AND btrim(basis_ref) <> ''
        )
);

CREATE TABLE access_identity.integration_client_grant (
    tenant_id           text        NOT NULL,
    grant_id            text        NOT NULL,
    issuer              text        NOT NULL,
    subject             text        NOT NULL,
    fact_type           text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    basis_ref           text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT integration_client_grant_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT integration_client_grant_binding_fkey
        FOREIGN KEY (issuer, subject, tenant_id)
        REFERENCES access_identity.integration_client (issuer, subject, tenant_id),

    CONSTRAINT integration_client_grant_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(grant_id) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 与 accessidentity.ExternalFactType 同一份封闭集，两处同笔改。更正不另立一类（ADR-0151 决定四）。
    CONSTRAINT integration_client_grant_fact_type_decided
        CHECK (fact_type IN (
            'CUSTOMS_EXTERNAL_RESULT', 'REGULATORY_CREDENTIAL', 'CARRIER_TRACKING',
            'CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE', 'EXTERNAL_FUNDS_FACT'
        )),

    -- 含起点、不含终点，终点缺席即不设终点。
    CONSTRAINT integration_client_grant_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 铸造信封时那一查以客户端主体发问、要取名下全部授予。
CREATE INDEX integration_client_grant_by_subject
    ON access_identity.integration_client_grant (issuer, subject);

CREATE TABLE access_identity.integration_client_grant_revocation (
    tenant_id   text        NOT NULL,
    grant_id    text        NOT NULL,
    revoked_at  timestamptz NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 一笔授予至多撤一次：撤销是终局，撤了再授是另一笔授予。
    CONSTRAINT integration_client_grant_revocation_pkey PRIMARY KEY (tenant_id, grant_id),

    CONSTRAINT integration_client_grant_revocation_grant_fkey
        FOREIGN KEY (tenant_id, grant_id)
        REFERENCES access_identity.integration_client_grant (tenant_id, grant_id),

    CONSTRAINT integration_client_grant_revocation_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);
