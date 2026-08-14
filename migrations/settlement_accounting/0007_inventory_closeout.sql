-- 库存面收官：后续账期纳入、对账单异议、回收调整、索赔金额调整。
--
-- subsequent_inclusion 主键取（租户+纳入）：同一纳入标识只登一次。行上没有金额列
-- ——金额永远在费用/调整本体上（AT-SA-076/077）。既有调整必须指名调整；迟到费用
-- 不得指名。后续账期不得等于原周期。
--
-- statement_dispute 主键取（租户+异议）：开立走 Save，裁定走 Replace。Replace 只写
-- 裁定三列，不改对账单号/费用/争议金额。裁定三件同在或同缺。
--
-- recovery_adjustment / claim_amount_adjustment 主键取（租户+调整）：同一标识只
-- 形成一次。原金额不被改写。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

CREATE TABLE settlement_accounting.subsequent_inclusion (
    tenant_id           text        NOT NULL,
    inclusion_id        text        NOT NULL,

    kind                text        NOT NULL,
    statement_number    text        NOT NULL,
    original_period     text        NOT NULL,
    subsequent_period   text        NOT NULL,
    charge_id           text        NOT NULL,
    adjustment_id       text,
    included_at         timestamptz NOT NULL,
    content_digest      text        NOT NULL,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT subsequent_inclusion_pkey
        PRIMARY KEY (tenant_id, inclusion_id),

    CONSTRAINT subsequent_inclusion_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(inclusion_id) <> ''
            AND btrim(kind) <> ''
            AND btrim(statement_number) <> ''
            AND btrim(original_period) <> ''
            AND btrim(subsequent_period) <> ''
            AND btrim(charge_id) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT subsequent_inclusion_kind_closed
        CHECK (kind IN ('ADJUSTMENT', 'LATE_CHARGE')),

    CONSTRAINT subsequent_inclusion_period_forward
        CHECK (subsequent_period <> original_period),

    -- 既有调整必须指名调整；迟到费用不得指名。可空列先验 IS NULL / IS NOT NULL。
    CONSTRAINT subsequent_inclusion_kind_shaped
        CHECK (
            (kind = 'ADJUSTMENT'
             AND adjustment_id IS NOT NULL AND btrim(adjustment_id) <> '')
            OR (kind = 'LATE_CHARGE' AND adjustment_id IS NULL)
        )
);

CREATE TABLE settlement_accounting.statement_dispute (
    tenant_id           text        NOT NULL,
    dispute_id          text        NOT NULL,

    statement_number    text        NOT NULL,
    charge_id           text        NOT NULL,
    disputed_minor      bigint      NOT NULL,
    reason_ref          text        NOT NULL,
    opened_at           timestamptz NOT NULL,
    resolution          text,
    resolution_ref      text,
    resolved_at         timestamptz,
    content_digest      text        NOT NULL,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT statement_dispute_pkey
        PRIMARY KEY (tenant_id, dispute_id),

    CONSTRAINT statement_dispute_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(dispute_id) <> ''
            AND btrim(statement_number) <> ''
            AND btrim(charge_id) <> ''
            AND btrim(reason_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT statement_dispute_amount_positive
        CHECK (disputed_minor > 0),

    -- 裁定三件同在或同缺；等号碰上 NULL 给 NULL，必须先 IS NULL。
    CONSTRAINT statement_dispute_resolution_coupled
        CHECK (
            (resolution IS NULL AND resolution_ref IS NULL AND resolved_at IS NULL)
            OR (resolution IS NOT NULL
                AND resolution IN ('ACCEPTED', 'PARTIALLY_ACCEPTED', 'REJECTED', 'PENDING_REVIEW')
                AND resolution_ref IS NOT NULL AND btrim(resolution_ref) <> ''
                AND resolved_at IS NOT NULL
                AND resolved_at >= opened_at)
        )
);

CREATE TABLE settlement_accounting.recovery_adjustment (
    tenant_id           text        NOT NULL,
    adjustment_id       text        NOT NULL,

    recovery_id         text        NOT NULL,
    reason              text        NOT NULL,
    new_basis           text        NOT NULL,
    direction           text        NOT NULL,
    currency            text        NOT NULL,
    amount_minor        bigint      NOT NULL,
    period_ref          text        NOT NULL,
    formed_at           timestamptz NOT NULL,
    content_digest      text        NOT NULL,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT recovery_adjustment_pkey
        PRIMARY KEY (tenant_id, adjustment_id),

    CONSTRAINT recovery_adjustment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(adjustment_id) <> ''
            AND btrim(recovery_id) <> ''
            AND btrim(reason) <> ''
            AND btrim(new_basis) <> ''
            AND btrim(direction) <> ''
            AND btrim(currency) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT recovery_adjustment_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT recovery_adjustment_reason_closed
        CHECK (reason IN (
            'TAX_ASSESSMENT_CORRECTED',
            'FUNDS_FACT_REVISED',
            'CUSTOMER_RESPONSIBILITY_CHANGED'
        )),

    CONSTRAINT recovery_adjustment_direction_closed
        CHECK (direction IN ('DEBIT', 'CREDIT'))
);

CREATE TABLE settlement_accounting.claim_amount_adjustment (
    tenant_id           text        NOT NULL,
    adjustment_id       text        NOT NULL,

    target_kind         text        NOT NULL,
    target_ref          text        NOT NULL,
    reason              text        NOT NULL,
    basis_ref           text        NOT NULL,
    direction           text        NOT NULL,
    currency            text        NOT NULL,
    amount_minor        bigint      NOT NULL,
    period_ref          text        NOT NULL,
    formed_at           timestamptz NOT NULL,
    content_digest      text        NOT NULL,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT claim_amount_adjustment_pkey
        PRIMARY KEY (tenant_id, adjustment_id),

    CONSTRAINT claim_amount_adjustment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(adjustment_id) <> ''
            AND btrim(target_kind) <> ''
            AND btrim(target_ref) <> ''
            AND btrim(reason) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(direction) <> ''
            AND btrim(currency) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT claim_amount_adjustment_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT claim_amount_adjustment_target_closed
        CHECK (target_kind IN (
            'CUSTOMER_CLAIM_AMOUNT',
            'RECOVERY_RECEIVABLE',
            'ACKNOWLEDGEMENT'
        )),

    CONSTRAINT claim_amount_adjustment_reason_closed
        CHECK (reason IN (
            'RESPONSIBILITY_REVISED',
            'AMOUNT_RULE_CORRECTED',
            'ACKNOWLEDGEMENT_CHANGED'
        )),

    CONSTRAINT claim_amount_adjustment_direction_closed
        CHECK (direction IN ('DEBIT', 'CREDIT'))
);
