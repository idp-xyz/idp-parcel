-- 商业结算政策册（ADR-0044）：一行记一个结算政策版本的方式与六维适用范围。
--
-- 与版本册分表，不复制版本壳、不存 scope_ref：理由与价格政策册相同。SettlementApplicability
-- 六维全部平铺成列，不进 jsonb——「同一精确范围两法命中即冲突」要按维度比对，埋进
-- jsonb 之后每次冲突判定都要先解一次文档。
--
-- method 只收 PREPAID / TERMS。第三值「客户级默认」在领域里被排除过一次，库上再开
-- 一格就是把它请回来。
--
-- object_kind CHECK = 7（SettlementPolicyObject）。
--
-- 族 A 不设「未配置」标记列：缺席由查无此行表达，解析译「无适用依据」。

CREATE TABLE party_commercial.commercial_settlement_policy (
    tenant_id           text        NOT NULL,
    object_kind         smallint    NOT NULL,
    object_id           text        NOT NULL,
    version_label       text        NOT NULL,

    method              text        NOT NULL,
    legal_entity_ref    text        NOT NULL,
    counterparty_ref    text        NOT NULL,
    contract_label      text        NOT NULL,
    charge_scope_ref    text        NOT NULL,
    currency_code       text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    registered_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_settlement_policy_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_settlement_policy_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_settlement_policy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(counterparty_ref) <> ''
            AND btrim(contract_label) <> ''
            AND btrim(charge_scope_ref) <> ''
            AND btrim(currency_code) <> ''
        ),

    CONSTRAINT commercial_settlement_policy_settlement_only
        CHECK (object_kind = 7),

    CONSTRAINT commercial_settlement_policy_method_closed
        CHECK (method IN ('PREPAID', 'TERMS')),

    CONSTRAINT commercial_settlement_policy_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);
