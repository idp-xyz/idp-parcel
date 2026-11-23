-- 报价的发出记录。只增不改，没有状态列。
--
-- 一条记录指名一份已固定的销售价格内容（方案标识、方案版本、规范化版本与内容摘要）、
-- 相对方、责任法人、收口的有效期、条款、发出人、时刻与方式。文件不进这张表：再导出按摘要
-- 重新投影。送达也不在这里记。
--
-- 本迁移不回填，也不插默认行。

CREATE TABLE party_commercial.quotation_send (
    tenant_id                 text        NOT NULL,
    quotation_id              text        NOT NULL,
    price_lane                text        NOT NULL,
    plan_id                   text        NOT NULL,
    plan_version              text        NOT NULL,
    canonicalization_version  text        NOT NULL,
    content_digest            text        NOT NULL,
    counterparty_party_id     text        NOT NULL,
    counterparty_name         text        NOT NULL,
    legal_entity_id           text        NOT NULL,
    legal_entity_name         text        NOT NULL,
    valid_from                timestamptz NOT NULL,
    valid_until               timestamptz NOT NULL,
    terms                     text        NOT NULL,
    sent_by                   text        NOT NULL,
    sent_at                   timestamptz NOT NULL,
    method                    text        NOT NULL,

    CONSTRAINT quotation_send_pkey
        PRIMARY KEY (tenant_id, quotation_id),

    CONSTRAINT quotation_send_lane_closed
        CHECK (price_lane IN ('APPROVED_DRAFT', 'REGISTERED_PRICE_CARD')),

    CONSTRAINT quotation_send_method_workbook
        CHECK (method = 'WORKBOOK_EXPORT'),

    CONSTRAINT quotation_send_period_closed
        CHECK (valid_until > valid_from),

    CONSTRAINT quotation_send_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND tenant_id = btrim(tenant_id)
            AND btrim(quotation_id) <> ''
            AND quotation_id = btrim(quotation_id)
            AND btrim(plan_id) <> ''
            AND plan_id = btrim(plan_id)
            AND btrim(plan_version) <> ''
            AND plan_version = btrim(plan_version)
            AND btrim(canonicalization_version) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(counterparty_party_id) <> ''
            AND btrim(counterparty_name) <> ''
            AND btrim(legal_entity_id) <> ''
            AND btrim(legal_entity_name) <> ''
            AND btrim(sent_by) <> ''
            AND terms = btrim(terms)
            AND terms !~ '[[:cntrl:]]'
        )
);

-- 列表按发出时刻倒序，同一时刻再按报价号倒序。
CREATE INDEX quotation_send_by_sent_at
    ON party_commercial.quotation_send (tenant_id, sent_at DESC, quotation_id DESC);
