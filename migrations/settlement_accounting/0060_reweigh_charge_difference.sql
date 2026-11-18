-- 后一代预估替代仍未确认的前一代，以及结算账户采用「实测差异自动调整」。
-- 被替代的费用行不改数字。没采用的账户不在采用册里。本迁移不种任何租户的行。

CREATE TABLE settlement_accounting.customer_charge_supersession (
    tenant_id               text        NOT NULL,
    charge_id               text        NOT NULL,
    superseded_by_charge_id text        NOT NULL,
    recorded_at             timestamptz NOT NULL,

    CONSTRAINT customer_charge_supersession_pkey
        PRIMARY KEY (tenant_id, charge_id),

    CONSTRAINT customer_charge_supersession_charge_fk
        FOREIGN KEY (tenant_id, charge_id)
        REFERENCES settlement_accounting.customer_charge (tenant_id, charge_id),

    CONSTRAINT customer_charge_supersession_successor_fk
        FOREIGN KEY (tenant_id, superseded_by_charge_id)
        REFERENCES settlement_accounting.customer_charge (tenant_id, charge_id),

    CONSTRAINT customer_charge_supersession_not_self
        CHECK (charge_id <> superseded_by_charge_id)
);

CREATE TABLE settlement_accounting.reweigh_adjustment_adoption (
    tenant_id   text        NOT NULL,
    account_id  text        NOT NULL,
    basis       text        NOT NULL,
    recorded_at timestamptz NOT NULL,

    CONSTRAINT reweigh_adjustment_adoption_pkey
        PRIMARY KEY (tenant_id, account_id),

    CONSTRAINT reweigh_adjustment_adoption_account_fk
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id),

    CONSTRAINT reweigh_adjustment_adoption_basis_present
        CHECK (btrim(basis) <> '')
);

ALTER TABLE settlement_accounting.sell_evaluation_request
    DROP CONSTRAINT sell_evaluation_request_stop_cause_matches_state;

ALTER TABLE settlement_accounting.sell_evaluation_request
    ADD CONSTRAINT sell_evaluation_request_stop_cause_matches_state
        CHECK (
            (state IN ('SUBMITTED', 'FORMED', 'ZERO_TOTAL') AND stop_cause IS NULL)
            OR (state = 'PENDING_JUDGMENT' AND COALESCE(stop_cause, '') IN (
                'EVALUATION_PENDING_JUDGMENT', 'AMOUNT_ROUNDING_UNDECLARED', 'FEE_ITEM_MAPPING_INCOMPLETE',
                'EVALUATION_CATALOGUE_UNRESOLVED', 'EVALUATION_SERIES_UNRESOLVED', 'EVALUATION_RATE_NOT_FOUND',
                'EVALUATION_INPUT_INCOMPLETE', 'PENDING_JUDGMENT_CAUSE_UNRECORDED',
                'REWEIGH_ADJUSTMENT_NOT_ADOPTED', 'REWEIGH_ADJUSTMENT_EXCEEDS_CHARGE'))
            OR (state = 'UNRATABLE' AND COALESCE(stop_cause, '') = 'EVALUATION_UNRATABLE')
            OR (state = 'CONFLICT' AND COALESCE(stop_cause, '') = 'EVALUATION_CONFLICT')
            OR (state = 'NOT_FORMED' AND COALESCE(stop_cause, '') IN (
                'EVALUATION_FAILED', 'PRICE_CARD_NOT_CONFIGURED', 'PRICE_CARD_APPLICABILITY_CONFLICT',
                'PRICING_INPUT_UNAVAILABLE'))
        );
