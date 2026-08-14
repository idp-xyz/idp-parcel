-- 客户费用确认、实际代垫评估与客户代垫回收。
--
-- customer_charge 主键取（租户+费用）：SaveConfirmed 落确认。已确认行不会被第二次
-- 确认覆盖——ON CONFLICT 后 WHERE stage IS DISTINCT FROM 'CONFIRMED'，零行译
-- `已确认`（ADR-0031）。确认两半同在或同缺。
--
-- advance_assessment 主键取（租户+评估）：同一评估标识只登一次。成立必须有资金事实
-- /付款方/责任且无负向依据；其余三值必须带依据。
--
-- advance_recovery 主键取（租户+回收）：同一回收标识只形成一次。金额正数；评估裁决
-- 与上限是写入时已经判过的，行上只钉回收自身形状。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

CREATE TABLE settlement_accounting.customer_charge (
    tenant_id           text        NOT NULL,
    charge_id           text        NOT NULL,

    fee_item            text        NOT NULL,
    evaluation_ref      text        NOT NULL,
    currency            text        NOT NULL,
    amount_minor        bigint      NOT NULL,
    stage               text        NOT NULL,
    confirmation_basis  text,
    formed_at           timestamptz NOT NULL,
    confirmed_at        timestamptz,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_charge_pkey
        PRIMARY KEY (tenant_id, charge_id),

    CONSTRAINT customer_charge_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(charge_id) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(evaluation_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(stage) <> ''
        ),

    CONSTRAINT customer_charge_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT customer_charge_stage_closed
        CHECK (stage IN ('ESTIMATED', 'PROVISIONAL', 'CONFIRMED')),

    -- 确认两半同在或同缺；等号碰上 NULL 给 NULL，必须先 IS NULL。
    CONSTRAINT customer_charge_confirmation_coupled
        CHECK (
            (stage <> 'CONFIRMED'
             AND confirmation_basis IS NULL
             AND confirmed_at IS NULL)
            OR (stage = 'CONFIRMED'
                AND confirmation_basis IS NOT NULL AND btrim(confirmation_basis) <> ''
                AND confirmed_at IS NOT NULL
                AND confirmed_at >= formed_at)
        )
);

CREATE TABLE settlement_accounting.advance_assessment (
    tenant_id          text        NOT NULL,
    assessment_id      text        NOT NULL,

    obligation_ref     text        NOT NULL,
    verdict            text        NOT NULL,
    funds_fact         text,
    payer_ref          text,
    responsibility_ref text,
    basis              text,
    currency           text        NOT NULL,
    amount_minor       bigint      NOT NULL,
    version            text        NOT NULL,
    judged_at          timestamptz NOT NULL,
    content_digest     text        NOT NULL,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT advance_assessment_pkey
        PRIMARY KEY (tenant_id, assessment_id),

    CONSTRAINT advance_assessment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(assessment_id) <> ''
            AND btrim(obligation_ref) <> ''
            AND btrim(verdict) <> ''
            AND btrim(currency) <> ''
            AND btrim(version) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT advance_assessment_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT advance_assessment_verdict_closed
        CHECK (verdict IN ('ESTABLISHED', 'NOT_ESTABLISHED', 'UNDECIDED', 'CONFLICTING')),

    CONSTRAINT advance_assessment_verdict_shaped
        CHECK (
            (verdict = 'ESTABLISHED'
             AND funds_fact IS NOT NULL AND btrim(funds_fact) <> ''
             AND payer_ref IS NOT NULL AND btrim(payer_ref) <> ''
             AND responsibility_ref IS NOT NULL AND btrim(responsibility_ref) <> ''
             AND basis IS NULL)
            OR (verdict IN ('NOT_ESTABLISHED', 'UNDECIDED', 'CONFLICTING')
                AND basis IS NOT NULL AND btrim(basis) <> '')
        )
);

CREATE TABLE settlement_accounting.advance_recovery (
    tenant_id          text        NOT NULL,
    recovery_id        text        NOT NULL,

    assessment_id      text        NOT NULL,
    customer_ref       text        NOT NULL,
    contract_basis     text        NOT NULL,
    account_id         text        NOT NULL,
    currency           text        NOT NULL,
    amount_minor       bigint      NOT NULL,
    formed_at          timestamptz NOT NULL,
    content_digest     text        NOT NULL,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT advance_recovery_pkey
        PRIMARY KEY (tenant_id, recovery_id),

    CONSTRAINT advance_recovery_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(recovery_id) <> ''
            AND btrim(assessment_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(contract_basis) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT advance_recovery_amount_positive
        CHECK (amount_minor > 0)
);
