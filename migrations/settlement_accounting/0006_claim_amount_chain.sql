-- 客户方向索赔金额、应追偿与追偿认可。
--
-- customer_claim_amount 主键取（租户+金额）：同一标识只形成一次。赔付义务不得指名
-- 原费用，索赔费用退款必须指名——两族不混（AT-SA-153）由 CHECK 钉在库面。
--
-- recovery_receivable 主键取（租户+应追偿）：责任条件满足即可独立形成，行上不承载
-- 认可或到账（AT-SA-149）。
--
-- recovery_acknowledgement 主键取（租户+认可）：全部接受必须等额，部分接受必须少
-- 于应追偿。认可不是到账——行上没有收款或核销列。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

CREATE TABLE settlement_accounting.customer_claim_amount (
    tenant_id            text        NOT NULL,
    amount_id            text        NOT NULL,

    kind                 text        NOT NULL,
    claim_item           text        NOT NULL,
    responsibility_ref   text        NOT NULL,
    rule_version         text        NOT NULL,
    legal_entity         text        NOT NULL,
    original_charge      text,
    currency             text        NOT NULL,
    amount_minor         bigint      NOT NULL,
    period_ref           text        NOT NULL,
    formed_at            timestamptz NOT NULL,
    content_digest       text        NOT NULL,
    recorded_at          timestamptz NOT NULL,
    inserted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_claim_amount_pkey
        PRIMARY KEY (tenant_id, amount_id),

    CONSTRAINT customer_claim_amount_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(amount_id) <> ''
            AND btrim(kind) <> ''
            AND btrim(claim_item) <> ''
            AND btrim(responsibility_ref) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(currency) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT customer_claim_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT customer_claim_amount_kind_closed
        CHECK (kind IN ('COMPENSATION_PAYABLE', 'CLAIM_CHARGE_REFUND')),

    -- 退款必须指名原费用，赔付不得指名。可空列先验 IS NULL / IS NOT NULL。
    CONSTRAINT customer_claim_amount_kind_shaped
        CHECK (
            (kind = 'COMPENSATION_PAYABLE' AND original_charge IS NULL)
            OR (kind = 'CLAIM_CHARGE_REFUND'
                AND original_charge IS NOT NULL AND btrim(original_charge) <> '')
        )
);

CREATE TABLE settlement_accounting.recovery_receivable (
    tenant_id            text        NOT NULL,
    receivable_id        text        NOT NULL,

    matter_ref           text        NOT NULL,
    responsibility_ref   text        NOT NULL,
    counterparty_ref     text        NOT NULL,
    rule_version         text        NOT NULL,
    legal_entity         text        NOT NULL,
    currency             text        NOT NULL,
    amount_minor         bigint      NOT NULL,
    formed_at            timestamptz NOT NULL,
    content_digest       text        NOT NULL,
    recorded_at          timestamptz NOT NULL,
    inserted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT recovery_receivable_pkey
        PRIMARY KEY (tenant_id, receivable_id),

    CONSTRAINT recovery_receivable_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(receivable_id) <> ''
            AND btrim(matter_ref) <> ''
            AND btrim(responsibility_ref) <> ''
            AND btrim(counterparty_ref) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT recovery_receivable_amount_positive
        CHECK (amount_minor > 0)
);

CREATE TABLE settlement_accounting.recovery_acknowledgement (
    tenant_id            text        NOT NULL,
    acknowledgement_id   text        NOT NULL,

    receivable_id        text        NOT NULL,
    response_ref         text        NOT NULL,
    standing             text        NOT NULL,
    currency             text        NOT NULL,
    acknowledged_minor   bigint      NOT NULL,
    receivable_minor     bigint      NOT NULL,
    acknowledged_at      timestamptz NOT NULL,
    content_digest       text        NOT NULL,
    recorded_at          timestamptz NOT NULL,
    inserted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT recovery_acknowledgement_pkey
        PRIMARY KEY (tenant_id, acknowledgement_id),

    CONSTRAINT recovery_acknowledgement_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(acknowledgement_id) <> ''
            AND btrim(receivable_id) <> ''
            AND btrim(response_ref) <> ''
            AND btrim(standing) <> ''
            AND btrim(currency) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT recovery_acknowledgement_standing_closed
        CHECK (standing IN ('ACCEPTED', 'PARTIALLY_ACCEPTED')),

    CONSTRAINT recovery_acknowledgement_amounts_positive
        CHECK (acknowledged_minor > 0 AND receivable_minor > 0),

    -- 全部接受必须等额，部分接受必须少于应追偿。两格不互借。
    CONSTRAINT recovery_acknowledgement_standing_shaped
        CHECK (
            (standing = 'ACCEPTED' AND acknowledged_minor = receivable_minor)
            OR (standing = 'PARTIALLY_ACCEPTED' AND acknowledged_minor < receivable_minor)
        )
);
