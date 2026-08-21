-- 价卡版本仓储：定价方案版本（含其价表版本与全部规则结构）的登记册。
--
-- 来源：SYN-WALL-DOOR-AUDIT 票 07——三价表族机制已实现，但一份 BUY/SELL 价卡没有任何
-- 地方可放。本迁移只建仓储结构，不种任何默认行：价卡内容全属实例半边（PAR-SET-02/03
-- 待提供），机制先行（AGENTS.md：机制现在就做，实例留空并拒绝默认值）。
--
-- 键 =（租户 + 方案 + 方案版本）。「发布后的定价方案、价表、规则工件和版本清单不可
-- 原地覆盖」（CONTEXT）——行只增不改，同键第二份由主键拦住，适配器按（规范化版本 +
-- 内容摘要）比对译成`已登记`或`版本内容冲突`：版本引用相同、规范化版本也相同而内容
-- 摘要不同，视为版本内容冲突，不得继续重放或静默替换；规范化版本不同摘要不可比，
-- 不是冲突（ADR-0014），交治理裁决。
--
-- 列面只是比对与检索（方向隔离、按适用范围与计价基准时点选卡）；权威内容在 snapshot
-- ——领域折装的登记快照（方案全图 + 源文件身份 + 方向授权引用 + 发布批准责任方），
-- 读回经领域整图重验（含按规范化版本重算内容摘要自校）。
--
-- 源文件身份与 SHA-256 是证据索引：真实价卡文件外置于受限证据库（ADR-0008，敏感实例
-- 外置红线），仓储只登名称与哈希，供争议时指回可复核的源。

CREATE TABLE parcel_pricing.price_card_version (
    tenant_id             text        NOT NULL,
    plan_id               text        NOT NULL,
    plan_version          text        NOT NULL,

    -- 价格方向隔离：BUY 价卡的存在不推导 SELL 侧任何结论，装载必须按方向过滤。
    -- 首发方向与计算目的成对声明（CONTEXT），两列都存供查询，配对由领域门守。
    direction             text        NOT NULL,
    purpose               text        NOT NULL,
    scope                 text        NOT NULL,

    -- 方案携带的基础价表版本，单列冗余供追溯查询；附加费价表在快照内。
    rate_table_id         text        NOT NULL,
    rate_table_version    text        NOT NULL,

    -- 适用期 [effective_from, effective_to)，effective_to 为 NULL 表示无上界。
    effective_from        timestamptz NOT NULL,
    effective_to          timestamptz,

    -- 摘要只在同一规范化版本内可比（ADR-0014）。
    canonicalization      text        NOT NULL,
    content_digest        text        NOT NULL,

    source_file_name      text        NOT NULL,
    source_file_sha256    text        NOT NULL,

    -- BUY/SELL 方向授权引用：party-commercial 签发的授权工件（版本引用），这里只登
    -- 引用不解析内容——解析属授权工件的所有者。
    authorization_id      text        NOT NULL,
    authorization_version text        NOT NULL,

    -- 发布批准责任方：生命周期「已校验 → 已批准」的责任归属。
    publication_approver  text        NOT NULL,

    snapshot              jsonb       NOT NULL,
    registered_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT price_card_version_pkey
        PRIMARY KEY (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_direction_closed
        CHECK (direction IN ('BUY', 'SELL', 'INTERNAL')),
    CONSTRAINT price_card_version_purpose_closed
        CHECK (purpose IN ('CUSTOMER_CHARGE', 'SUPPLIER_COST', 'INTERNAL_PRICE')),

    CONSTRAINT price_card_version_sha256_shape
        CHECK (source_file_sha256 ~ '^[0-9a-f]{64}$'),

    CONSTRAINT price_card_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from),

    CONSTRAINT price_card_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(plan_id) <> ''
            AND btrim(plan_version) <> ''
            AND btrim(scope) <> ''
            AND btrim(rate_table_id) <> ''
            AND btrim(rate_table_version) <> ''
            AND btrim(canonicalization) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(source_file_name) <> ''
            AND btrim(authorization_id) <> ''
            AND btrim(authorization_version) <> ''
            AND btrim(publication_approver) <> ''
        )
);
