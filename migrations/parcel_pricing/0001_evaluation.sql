-- 计价评价登记册：评价是不可变的版本化计算事实，键=评价标识；失败评价照存——失败
-- 是计算事实不是丢弃品。同键第二份由主键拦住，适配器以 ON CONFLICT DO NOTHING 译成
-- `已有记录`。
--
-- 语义摘要、状态、方案内容摘要与规范化版本单列冗余存放：冒名判定在应用层拿语义摘要
-- 比、重放可比性只在同一规范化版本内成立（ADR-0014）——列存供查询与比对，权威内容
-- 在 snapshot（领域折装的完整快照，读回经领域整图重验含摘要自校）。

CREATE TABLE parcel_pricing.evaluation (
    evaluation_id        text        NOT NULL,

    tenant_id            text        NOT NULL,
    status               text        NOT NULL,
    semantic_digest      text        NOT NULL,
    plan_content_digest  text        NOT NULL,
    canonicalization     text        NOT NULL,
    snapshot             jsonb       NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT evaluation_pkey PRIMARY KEY (evaluation_id),

    CONSTRAINT evaluation_not_blank
        CHECK (
            btrim(evaluation_id) <> ''
            AND btrim(tenant_id) <> ''
            AND btrim(semantic_digest) <> ''
            AND btrim(plan_content_digest) <> ''
            AND btrim(canonicalization) <> ''
        ),
    CONSTRAINT evaluation_status_closed
        CHECK (status IN ('COMPLETED', 'PENDING', 'CONFLICT', 'FAILED', 'UNRATABLE'))
);
