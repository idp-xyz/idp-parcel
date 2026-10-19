-- SELL 计费时点采用册（ADR-0190 决定三）：键是（租户、结算账户），值是（计费时点、计价范围），不设修订，照受理前估价
-- 采用册的不可变元组。一户只采用一种时点由主键守。指向的结算账户必须已在账户册上，本册不代为建账户。
-- 不预选时点，不登任何租户的行。

CREATE TABLE settlement_accounting.sell_billing_moment_adoption (
    tenant_id        text        NOT NULL,
    account_id       text        NOT NULL,
    billing_moment   text        NOT NULL,
    pricing_scope_id text        NOT NULL,
    registered_at    timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sell_billing_moment_adoption_pkey
        PRIMARY KEY (tenant_id, account_id),

    CONSTRAINT sell_billing_moment_adoption_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(pricing_scope_id) <> ''
        ),

    CONSTRAINT sell_billing_moment_adoption_moment_closed
        CHECK (billing_moment IN ('NETWORK_INTAKE_ADOPTED', 'EFFECTIVE_DELIVERY_FINALIZED')),

    CONSTRAINT sell_billing_moment_adoption_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id)
);
