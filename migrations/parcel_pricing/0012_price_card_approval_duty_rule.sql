-- 价卡发布审批职责规则（ADR-0101 决定六；spec 自决第 2 格；票 price-card-import/04）：租户对「已校验 → 已批准」这一步
-- 的治理规则，一租户一条——录入者与批准者须否为不同主体、批准者须持哪一格授予。两格都不要求也是一条合法的租户声明。
--
-- 形状同 party_commercial.publication_approval_duty_rule，但不共用那一张：租户对价卡与对商业版本的审批要求可以不同。
-- 行属实例半边（参数登记册「价卡发布审批职责规则」）：本迁移不种任何行；没有行时批准门答`未配置`、不放行，不以
-- 任何默认代替。写口今天只给装配与测试用。
--
-- 推进草稿（批准、发布）只改 price_card_draft 上 0011 已立的状态与痕迹列，本迁移不动那张表。

CREATE TABLE parcel_pricing.price_card_approval_duty_rule (
    tenant_id                  text        NOT NULL,
    requires_distinct_subjects boolean     NOT NULL,
    -- NULL 即不要求授予。
    required_grant             text,
    registered_at              timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT price_card_approval_duty_rule_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT price_card_approval_duty_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND (required_grant IS NULL OR btrim(required_grant) <> '')
        )
);
