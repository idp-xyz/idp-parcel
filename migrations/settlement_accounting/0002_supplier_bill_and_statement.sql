-- 供应商账单接收与已发布对账单：UC-SA-004 / UC-SA-003 的两张判断库。
--
-- supplier_bill_reception 主键取幂等三维（租户+主张身份+版本）：相同主张与版本重复
-- 到达返回原结果，同一身份内容变化是版本冲突（由应用按 content_digest 判）；撞键即
-- `已有记录`（ON CONFLICT 代数，ADR-0031），原始主张不可覆盖。
--
-- customer_statement 主键取（租户+单号）：同一发布意图恰一个单号。发布密封：内容列
-- 只在 INSERT 落，作废走 Replace 只写留痕两列（void_basis/voided_at）——作废不删行
-- 不改总额，替代单用新单号（AT-SA-069）。

CREATE TABLE settlement_accounting.supplier_bill_reception (
    tenant_id                  text        NOT NULL,
    claim_id                   text        NOT NULL,
    claim_version              text        NOT NULL,

    supplier_ref               text        NOT NULL,
    legal_entity               text        NOT NULL,
    period_ref                 text        NOT NULL,
    currency                   text        NOT NULL,

    content_digest             text        NOT NULL,
    claim                      jsonb       NOT NULL,
    matches                    jsonb       NOT NULL,
    audit_authority_configured boolean     NOT NULL,
    recorded_at                timestamptz NOT NULL,

    CONSTRAINT supplier_bill_reception_pkey
        PRIMARY KEY (tenant_id, claim_id, claim_version),

    CONSTRAINT supplier_bill_reception_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(claim_id) <> ''
            AND btrim(claim_version) <> ''
            AND btrim(supplier_ref) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 没有行的主张不存在（领域 ReceiveSupplierBillClaim 的库面）。
    CONSTRAINT supplier_bill_reception_lines_present
        CHECK (
            jsonb_typeof(claim -> 'lines') = 'array'
            AND jsonb_array_length(claim -> 'lines') > 0
        ),
    CONSTRAINT supplier_bill_reception_matches_shaped
        CHECK (jsonb_typeof(matches) = 'array')
);

CREATE TABLE settlement_accounting.customer_statement (
    tenant_id        text        NOT NULL,
    statement_number text        NOT NULL,

    account_id       text        NOT NULL,
    period_ref       text        NOT NULL,
    currency         text        NOT NULL,
    total_minor      bigint      NOT NULL,

    content_digest   text        NOT NULL,
    lines            jsonb       NOT NULL,
    adjustment_lines jsonb       NOT NULL,
    published_at     timestamptz NOT NULL,
    void_basis       text,
    voided_at        timestamptz,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT customer_statement_pkey
        PRIMARY KEY (tenant_id, statement_number),

    CONSTRAINT customer_statement_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(statement_number) <> ''
            AND btrim(account_id) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 没有行的对账单不存在；行与总额列 NOT NULL——作废留痕不是删内容。
    CONSTRAINT customer_statement_lines_present
        CHECK (jsonb_typeof(lines) = 'array' AND jsonb_array_length(lines) > 0),
    CONSTRAINT customer_statement_adjustments_shaped
        CHECK (jsonb_typeof(adjustment_lines) = 'array'),

    -- 作废留痕（AT-SA-069）：依据与时刻同在或同缺，作废不早于发布。
    CONSTRAINT customer_statement_void_trace_coherent
        CHECK (
            (void_basis IS NULL AND voided_at IS NULL)
            OR (void_basis IS NOT NULL AND btrim(void_basis) <> ''
                AND voided_at IS NOT NULL AND voided_at >= published_at)
        )
);
