-- 推送源册与重放账（ADR-0149 决定三第三条、决定四第一条；票 operator-channel/12）：只能推送的外部源绑在集成客户端册的
-- 一个客户端主体上，名下的签名密钥只存受控引用；重放账记已经收下的签名原文。册的列由产品定；行是租户取值（参数登记册
-- PAR-INT-12），本迁移不种任何行，也不放任何密钥。
--
-- 三张登记表只增不改，理由同 0001：停用是另一张表里的一行，被停的钥是那段时间里它确实有效的证据。
--
-- 库里只登标识与密钥引用：HMAC 共享密钥与对方公钥本体都不落库（ADR-0149 越权风险点 4），引用只指向租户约定的受限
-- 存储里那一份的出处。公钥本身不是秘密，仍只存引用，是为了两形走同一条取用与轮换路径。

CREATE TABLE access_identity.push_source (
    source_id          text        NOT NULL,
    tenant_id          text        NOT NULL,
    issuer             text        NOT NULL,
    subject            text        NOT NULL,
    window_seconds     bigint      NOT NULL,
    clock_skew_seconds bigint      NOT NULL,
    basis_ref          text        NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),

    -- 键不含租户：同一个推送源标识至多一行。推送不说自己属哪个租户，租户取自认领到的这一行。
    CONSTRAINT push_source_pkey PRIMARY KEY (source_id),

    -- 密钥与重放账经这两列回指绑定：它们的租户必须就是推送源所绑的那个。
    CONSTRAINT push_source_tenant_binding_key UNIQUE (source_id, tenant_id),

    CONSTRAINT push_source_client_fkey
        FOREIGN KEY (issuer, subject, tenant_id)
        REFERENCES access_identity.integration_client (issuer, subject, tenant_id),

    CONSTRAINT push_source_not_blank
        CHECK (
            btrim(source_id) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(issuer) <> ''
            AND btrim(subject) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 窗口是签名时刻最多可早于收到时刻多久，必须给出且按整秒；时钟偏差是最多可晚于收到时刻多久，可以为零。
    -- 两件都是租户取值，产品不设缺省，所以这里不写 DEFAULT。
    CONSTRAINT push_source_window_decided
        CHECK (window_seconds > 0 AND clock_skew_seconds >= 0)
);

CREATE TABLE access_identity.push_signing_key (
    tenant_id           text        NOT NULL,
    source_id           text        NOT NULL,
    key_id              text        NOT NULL,
    form                text        NOT NULL,
    key_ref             text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    basis_ref           text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT push_signing_key_pkey PRIMARY KEY (tenant_id, source_id, key_id),

    CONSTRAINT push_signing_key_source_fkey
        FOREIGN KEY (source_id, tenant_id)
        REFERENCES access_identity.push_source (source_id, tenant_id),

    CONSTRAINT push_signing_key_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(key_id) <> ''
            AND btrim(key_ref) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 与 accessidentity.PushSignatureForm 同一份封闭集，两处同笔改。
    CONSTRAINT push_signing_key_form_decided
        CHECK (form IN ('HMAC_SHA256', 'ED25519')),

    -- 含起点、不含终点，终点缺席即不设终点。
    CONSTRAINT push_signing_key_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 铸造时按推送源取名下全部密钥。
CREATE INDEX push_signing_key_by_source
    ON access_identity.push_signing_key (source_id);

CREATE TABLE access_identity.push_signing_key_retirement (
    tenant_id   text        NOT NULL,
    source_id   text        NOT NULL,
    key_id      text        NOT NULL,
    retires_at  timestamptz NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 一把钥至多停一次：停用是终局，停了再启用是另一把钥。
    CONSTRAINT push_signing_key_retirement_pkey PRIMARY KEY (tenant_id, source_id, key_id),

    CONSTRAINT push_signing_key_retirement_key_fkey
        FOREIGN KEY (tenant_id, source_id, key_id)
        REFERENCES access_identity.push_signing_key (tenant_id, source_id, key_id),

    CONSTRAINT push_signing_key_retirement_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);

-- 重放账：同一推送源下同一份签名原文（SHA-256）至多一行。键不取推送标识——推送方的重试带新时间戳重签，原文不同，
-- 不在这里拦。保留到签名时刻加窗口即够，过了那一刻同一份原文先被越窗那一格拦下；清扫过期行不在本迁移里。
CREATE TABLE access_identity.push_receipt (
    source_id    text        NOT NULL,
    digest       text        NOT NULL,
    tenant_id    text        NOT NULL,
    push_id      text        NOT NULL,
    signed_at    timestamptz NOT NULL,
    retain_until timestamptz NOT NULL,
    recorded_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT push_receipt_pkey PRIMARY KEY (source_id, digest),

    CONSTRAINT push_receipt_source_fkey
        FOREIGN KEY (source_id, tenant_id)
        REFERENCES access_identity.push_source (source_id, tenant_id),

    CONSTRAINT push_receipt_not_blank
        CHECK (
            btrim(source_id) <> ''
            AND btrim(digest) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(push_id) <> ''
        ),

    CONSTRAINT push_receipt_retention_coherent
        CHECK (retain_until >= signed_at)
);
