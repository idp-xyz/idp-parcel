-- 外部资金事实、资金映射与核销。
--
-- external_funds_fact 主键取（租户+事实引用）：同一事实只采用一次。更正两半同在或
-- 同缺；更正版本不得等于当前版本。
--
-- funds_mapping 主键取（租户+映射）：同一映射标识只登一次。目标种类封闭；依据必备。
--
-- settlement_application 主键取（租户+核销）：Save 登记，Replace 只写撤销两列。
-- 分配 jsonb 非空数组；净额正且不超过事实金额。撤销两半同在或同缺。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取长度。

CREATE TABLE settlement_accounting.external_funds_fact (
    tenant_id      text        NOT NULL,
    fact_id        text        NOT NULL,

    source_ref     text        NOT NULL,
    kind           text        NOT NULL,
    currency       text        NOT NULL,
    amount_minor   bigint      NOT NULL,
    version        text        NOT NULL,
    occurred_at    timestamptz NOT NULL,
    corrects       text,
    corrected_at   timestamptz,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_funds_fact_pkey
        PRIMARY KEY (tenant_id, fact_id),

    CONSTRAINT external_funds_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(kind) <> ''
            AND btrim(currency) <> ''
            AND btrim(version) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT external_funds_fact_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT external_funds_fact_kind_closed
        CHECK (kind IN ('RECEIPT_CONFIRMED', 'PAYMENT_FAILED', 'FUNDS_RETURNED')),

    CONSTRAINT external_funds_fact_correction_coupled
        CHECK (
            (corrects IS NULL AND corrected_at IS NULL)
            OR (corrects IS NOT NULL AND btrim(corrects) <> ''
                AND corrects <> version
                AND corrected_at IS NOT NULL
                AND corrected_at >= occurred_at)
        )
);

CREATE TABLE settlement_accounting.funds_mapping (
    tenant_id      text        NOT NULL,
    mapping_id     text        NOT NULL,

    fact_id        text        NOT NULL,
    target_kind    text        NOT NULL,
    target_ref     text        NOT NULL,
    basis          text        NOT NULL,
    mapped_at      timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT funds_mapping_pkey
        PRIMARY KEY (tenant_id, mapping_id),

    CONSTRAINT funds_mapping_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(mapping_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(target_kind) <> ''
            AND btrim(target_ref) <> ''
            AND btrim(basis) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT funds_mapping_target_kind_closed
        CHECK (target_kind IN ('STATEMENT', 'PAYABLE', 'CREDIT_NOTE'))
);

CREATE TABLE settlement_accounting.settlement_application (
    tenant_id       text        NOT NULL,
    application_id  text        NOT NULL,

    fact_id         text        NOT NULL,
    currency        text        NOT NULL,
    fact_minor      bigint      NOT NULL,
    applied_minor   bigint      NOT NULL,
    allocations     jsonb       NOT NULL,
    basis           text        NOT NULL,
    applied_at      timestamptz NOT NULL,
    reversal_basis  text,
    reversed_at     timestamptz,
    content_digest  text        NOT NULL,
    recorded_at     timestamptz NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT settlement_application_pkey
        PRIMARY KEY (tenant_id, application_id),

    CONSTRAINT settlement_application_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(application_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(basis) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT settlement_application_amounts_conserved
        CHECK (
            fact_minor > 0
            AND applied_minor > 0
            AND applied_minor <= fact_minor
        ),

    CONSTRAINT settlement_application_allocations_shaped
        CHECK (
            allocations IS NOT NULL
            AND jsonb_typeof(allocations) = 'array'
            AND jsonb_array_length(allocations) > 0
        ),

    CONSTRAINT settlement_application_reversal_coupled
        CHECK (
            (reversal_basis IS NULL AND reversed_at IS NULL)
            OR (reversal_basis IS NOT NULL AND btrim(reversal_basis) <> ''
                AND reversed_at IS NOT NULL
                AND reversed_at >= applied_at)
        )
);
