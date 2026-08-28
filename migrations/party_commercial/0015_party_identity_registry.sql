-- 参与方身份与关系登记册（CONTEXT「参与方身份」节 + ADR-0003 三级边界）：
-- 业务参与方、责任法人、货主客户账户三本身份册与参与方关系册。
--
-- 不并进 commercial_version：CommercialObjectKind 注释明写货主客户账户与责任法人
-- 刻意不在商业对象之中——它们是参与方身份，走自己的生命周期（登记→生效→停用），
-- 不是草稿→发布→退役。四表键=租户+对象标识+修订，修订从 1 起连续递增；内容更正
-- 与停用都是新修订行，绝不 UPDATE（ADR-0031 登记面纪律）。
--
-- 身份三表不存状态列：已登记/已生效/已停用由 effective_from 与 deactivated_at 对
-- 时点导出（domain.IdentityLifecycle.StatusAt），存状态列等于存一份会过期的推导。
-- 关系表存状态列：候选/已生效/已终止是登记进来的事实，不随时钟走。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL /
-- IS NOT NULL，没有一条比较式能单独以 NULL 决定约束。

CREATE TABLE party_commercial.business_party_registration (
    tenant_id          text        NOT NULL,
    party_id           text        NOT NULL,
    revision           integer     NOT NULL,

    party_name         text        NOT NULL,
    basis_ref          text        NOT NULL,
    effective_from     timestamptz NOT NULL,
    deactivated_at     timestamptz,
    deactivation_basis text,
    content_digest     text        NOT NULL,
    snapshot           jsonb       NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT business_party_registration_pkey
        PRIMARY KEY (tenant_id, party_id, revision),

    CONSTRAINT business_party_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(party_id) <> ''
            AND btrim(party_name) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT business_party_registration_revision_positive
        CHECK (revision >= 1),

    -- 停用两件（时点+依据）同进同出：只有其一的行说不出「依据什么停用」或「何时起停用」。
    CONSTRAINT business_party_registration_deactivation_paired
        CHECK (
            (deactivated_at IS NULL AND deactivation_basis IS NULL)
            OR (deactivated_at IS NOT NULL
                AND deactivation_basis IS NOT NULL
                AND btrim(deactivation_basis) <> '')
        ),

    CONSTRAINT business_party_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

CREATE TABLE party_commercial.legal_entity_registration (
    tenant_id          text        NOT NULL,
    legal_entity_id    text        NOT NULL,
    revision           integer     NOT NULL,

    -- 责任法人同时具有业务参与方身份（CONTEXT）；名称等身份内容在参与方册上，这里
    -- 只登关联，不抄第二份。悬空引用由写入用例把门，册间不设外键——登记册各自只增，
    -- 修订轴互相独立。
    party_id           text        NOT NULL,
    basis_ref          text        NOT NULL,
    effective_from     timestamptz NOT NULL,
    deactivated_at     timestamptz,
    deactivation_basis text,
    content_digest     text        NOT NULL,
    snapshot           jsonb       NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT legal_entity_registration_pkey
        PRIMARY KEY (tenant_id, legal_entity_id, revision),

    CONSTRAINT legal_entity_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity_id) <> ''
            AND btrim(party_id) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT legal_entity_registration_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT legal_entity_registration_deactivation_paired
        CHECK (
            (deactivated_at IS NULL AND deactivation_basis IS NULL)
            OR (deactivated_at IS NOT NULL
                AND deactivation_basis IS NOT NULL
                AND btrim(deactivation_basis) <> '')
        ),

    CONSTRAINT legal_entity_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

CREATE TABLE party_commercial.customer_account_registration (
    tenant_id          text        NOT NULL,
    account_id         text        NOT NULL,
    revision           integer     NOT NULL,

    customer_party_id  text        NOT NULL,
    basis_ref          text        NOT NULL,
    effective_from     timestamptz NOT NULL,
    deactivated_at     timestamptz,
    deactivation_basis text,
    content_digest     text        NOT NULL,
    snapshot           jsonb       NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_account_registration_pkey
        PRIMARY KEY (tenant_id, account_id, revision),

    CONSTRAINT customer_account_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(customer_party_id) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT customer_account_registration_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT customer_account_registration_deactivation_paired
        CHECK (
            (deactivated_at IS NULL AND deactivation_basis IS NULL)
            OR (deactivated_at IS NOT NULL
                AND deactivation_basis IS NOT NULL
                AND btrim(deactivation_basis) <> '')
        ),

    CONSTRAINT customer_account_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

CREATE TABLE party_commercial.party_relationship_registration (
    tenant_id             text        NOT NULL,
    relationship_id       text        NOT NULL,
    revision              integer     NOT NULL,

    holder_party_id       text        NOT NULL,
    counterparty_party_id text        NOT NULL,
    role                  text        NOT NULL,
    scope_ref             text        NOT NULL,
    basis_ref             text        NOT NULL,
    status                text        NOT NULL,
    effective_starts_at   timestamptz NOT NULL,
    effective_ends_at     timestamptz,
    approval_ref          text,
    approved_at           timestamptz,
    ended_at              timestamptz,
    end_basis             text,
    successor_party_id    text,
    content_digest        text        NOT NULL,
    snapshot              jsonb       NOT NULL,
    recorded_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT party_relationship_registration_pkey
        PRIMARY KEY (tenant_id, relationship_id, revision),

    CONSTRAINT party_relationship_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(relationship_id) <> ''
            AND btrim(holder_party_id) <> ''
            AND btrim(counterparty_party_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT party_relationship_registration_revision_positive
        CHECK (revision >= 1),

    -- 自己与自己不成关系（domain.NewCandidateRelationship 同一条门）。
    CONSTRAINT party_relationship_registration_two_parties
        CHECK (holder_party_id <> counterparty_party_id),

    -- 镜像 domain.PartyRole 封闭集。新增角色先改领域封闭集，再改这一条。
    CONSTRAINT party_relationship_registration_role_closed
        CHECK (role IN ('CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT', 'RESELLER', 'ACCOUNT_HOLDER')),

    -- 镜像 domain.RelationshipStatus 封闭集（CANDIDATE 之后的每一格都经真转换到达）。
    CONSTRAINT party_relationship_registration_status_closed
        CHECK (status IN ('CANDIDATE', 'EFFECTIVE', 'EXPIRED', 'REVOKED', 'SUPERSEDED')),

    CONSTRAINT party_relationship_registration_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at),

    -- 批准两件同进同出，且候选行不带批准、非候选行必带批准（Approve 是唯一出候选的门）。
    CONSTRAINT party_relationship_registration_approval_paired
        CHECK (
            (approval_ref IS NULL AND approved_at IS NULL AND status = 'CANDIDATE')
            OR (approval_ref IS NOT NULL AND btrim(approval_ref) <> ''
                AND approved_at IS NOT NULL AND status <> 'CANDIDATE')
        ),

    -- 终止事实与终止状态同进同出；撤销必带终止依据，替代必带接替方
    -- （domain.Revoke / SupersededBy 的镜像）。
    CONSTRAINT party_relationship_registration_end_coherent
        CHECK (
            (ended_at IS NULL AND end_basis IS NULL AND successor_party_id IS NULL
                AND status IN ('CANDIDATE', 'EFFECTIVE'))
            OR (ended_at IS NOT NULL AND status IN ('EXPIRED', 'REVOKED', 'SUPERSEDED')
                AND (status <> 'REVOKED' OR (end_basis IS NOT NULL AND btrim(end_basis) <> ''))
                AND (status <> 'SUPERSEDED'
                    OR (successor_party_id IS NOT NULL AND btrim(successor_party_id) <> '')))
        ),

    CONSTRAINT party_relationship_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

-- 目录按（租户+登记时间倒序）上列；身份三表的最新修订经主键即可定位，不另建索引。
CREATE INDEX party_relationship_registration_by_tenant
    ON party_commercial.party_relationship_registration
        (tenant_id, recorded_at DESC);
