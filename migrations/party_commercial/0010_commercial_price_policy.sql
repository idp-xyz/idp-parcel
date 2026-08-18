-- 商业价格政策册（ADR-0034 / ADR-0057）：一行记一个价格规则版本的计价正文。
--
-- 与版本册分表：版本回答「有没有这份价格规则对象」，政策回答「哪个方向绑了哪份定价
-- 方案」。行不自带版本快照，也不存 scope_ref——范围是版本壳上的事实，装载按版本行
-- 的 scope_ref 过滤。政策自己的适用范围是正文的一部分，另存 policy_scope_ref。
--
-- plan_direction 与 binding_conversion 不在 CommercialPricePolicy 结构体上：
-- NewCommercialPricePolicy 收下它们、交给 checkPlanBinding 判完即弃。不存这两列
-- 就重建不出政策。存的是发布当时 parcel-pricing 对该方案方向的答复，以及当时声明
-- 的转换；装载时原样交回构造函数，AT-PC-033 在装载面上也守得住（ADR-0057）。
--
-- object_kind CHECK = 6（PriceRuleObject）。形态只归价格规则。
--
-- 族 A 不设「未配置」标记列：ADR-0034 裁过只登记版本、不登记政策时计价落到
-- 「无适用依据」，缺席由查无此行表达。

CREATE TABLE party_commercial.commercial_price_policy (
    tenant_id           text        NOT NULL,
    object_kind         smallint    NOT NULL,
    object_id           text        NOT NULL,
    version_label       text        NOT NULL,

    direction           text        NOT NULL,
    plan_ref            text        NOT NULL,
    plan_direction      text        NOT NULL,
    binding_conversion  text        NOT NULL,
    policy_scope_ref    text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    registered_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_price_policy_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_price_policy_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT commercial_price_policy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(plan_ref) <> ''
            AND btrim(policy_scope_ref) <> ''
        ),

    CONSTRAINT commercial_price_policy_price_rule_only
        CHECK (object_kind = 6),

    CONSTRAINT commercial_price_policy_direction_closed
        CHECK (direction IN ('BUY', 'SELL', 'INTERNAL')),

    CONSTRAINT commercial_price_policy_plan_direction_closed
        CHECK (plan_direction IN ('BUY', 'SELL', 'INTERNAL')),

    CONSTRAINT commercial_price_policy_conversion_closed
        CHECK (binding_conversion IN ('NONE', 'FROZEN_BUY_EVALUATION')),

    -- 镜像 domain.checkPlanBinding：同向必须未声明转换；唯一合法的跨向是 SELL 政策
    -- 显式引用一次已冻结的 BUY 评价。独立枚举 CHECK 仍挡住集外取值，本约束挡住集内
    -- 非法组合——否则 SELL+BUY+NONE 能入册，要等到下次装载过 NewCommercialPricePolicy
    -- 才炸（ADR-0057）。
    CONSTRAINT commercial_price_policy_binding
        CHECK (
            (direction = plan_direction AND binding_conversion = 'NONE')
            OR (
                direction = 'SELL'
                AND plan_direction = 'BUY'
                AND binding_conversion = 'FROZEN_BUY_EVALUATION'
            )
        ),

    CONSTRAINT commercial_price_policy_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);
