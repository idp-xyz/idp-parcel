-- 授权治理册：版本化主动拒绝/人工复核授权（CONTEXT：主动拒绝授权必须按责任法人、
-- 角色、适用范围和有效期间版本化，并要求结构化原因和证据）。
--
-- 不并进 commercial_version：那张表的 object_kind CHECK 今天是 1..7，AuthorizationRule
-- 是第九类；且 CommercialVersion 没有动作、法人、权限等级。本册自带足以重建
-- AuthorityGrant 的快照。日后授权规则走发布登记册属另票（扩 object_kind 并接线），
-- 不在 0001 里留缺口。
--
-- action 的 CHECK 是 domain.AuthorizedAction 封闭集的镜像；扩展必须随该符号同步。
-- 今日两值：MANUAL_REVIEW、ACTIVE_REJECTION。撤回与资料修订不在集内，本表不预开空格。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL /
-- IS NOT NULL，没有一条比较式能单独以 NULL 决定约束。

CREATE TABLE party_commercial.authorization_grant (
    tenant_id           text        NOT NULL,
    object_id           text        NOT NULL,
    version_label       text        NOT NULL,

    action              text        NOT NULL,
    legal_entity_ref    text        NOT NULL,
    authority_level     text        NOT NULL,
    scope_ref           text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    content_digest      text        NOT NULL,
    snapshot            jsonb       NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT authorization_grant_pkey
        PRIMARY KEY (tenant_id, object_id, version_label),

    CONSTRAINT authorization_grant_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(authority_level) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 镜像 AuthorizedAction。新增动作先改领域封闭集，再改这一条。
    CONSTRAINT authorization_grant_action_closed
        CHECK (action IN ('MANUAL_REVIEW', 'ACTIVE_REJECTION')),

    CONSTRAINT authorization_grant_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at),

    CONSTRAINT authorization_grant_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

-- 裁定按（租户+范围+时点）装载当时管得着的授权。
CREATE INDEX authorization_grant_by_scope
    ON party_commercial.authorization_grant
        (tenant_id, scope_ref, effective_starts_at);
