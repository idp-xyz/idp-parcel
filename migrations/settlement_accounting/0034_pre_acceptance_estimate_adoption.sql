-- 受理前估价采用册（ADR-0179 决定五）：键是（租户、结算账户），值是（形态、计价范围），不设修订。首版形态只有一种。
-- 指向的结算账户必须已在账户册上，本册不代为建账户。不登任何租户的行。

CREATE TABLE settlement_accounting.pre_acceptance_estimate_adoption (
    tenant_id        text        NOT NULL,
    account_id       text        NOT NULL,
    form             text        NOT NULL,
    pricing_scope_id text        NOT NULL,
    registered_at    timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pre_acceptance_estimate_adoption_pkey
        PRIMARY KEY (tenant_id, account_id),

    CONSTRAINT pre_acceptance_estimate_adoption_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(pricing_scope_id) <> ''
        ),

    CONSTRAINT pre_acceptance_estimate_adoption_form_closed
        CHECK (form IN ('PER_PARCEL_SELL_TRIAL_SUM')),

    CONSTRAINT pre_acceptance_estimate_adoption_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id)
);
