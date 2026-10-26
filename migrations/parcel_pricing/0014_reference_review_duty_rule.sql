-- 参考复核职责规则（ADR-0262 决定一）：租户对计价参考序列与目录人工复核的治理规则，一租户一条，序列与目录共用——复核者须否
-- 与该版本的登记者为不同主体、复核者须持哪一格授予。两格都不要求也是一条合法的租户声明。
--
-- 形状同 0012 的 price_card_approval_duty_rule，但不共用那一张：租户对参考取值的转录复核与对价卡发布的批准，要求可以不同。
-- 行属实例半边：本迁移不种任何行；没有行时人工复核答`未配置`、不放行，不以任何默认代替。行只增不改——改规则不在这张表上
-- 原地覆盖，异内容的再登答冲突。
--
-- 复核行本身仍落在 0004 与 0008 的两张复核册上，本迁移不动它们：规则只决定复核落不落，不改复核记下的东西。

CREATE TABLE parcel_pricing.reference_review_duty_rule (
    tenant_id                  text        NOT NULL,
    requires_distinct_subjects boolean     NOT NULL,
    -- NULL 即不要求授予。
    required_grant             text,
    registered_at              timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reference_review_duty_rule_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT reference_review_duty_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND (required_grant IS NULL OR btrim(required_grant) <> '')
        )
);
