-- 一条费用项目的归属日判定：形态、业务时区、截单时刻。不预填时区或截单时刻。不登任何租户的行。不设修订。

CREATE TABLE settlement_accounting.charge_attribution (
    tenant_id      text        NOT NULL,
    fee_item       text        NOT NULL,
    form           text        NOT NULL,
    timezone_name  text        NOT NULL,
    cutoff_minute  integer     NOT NULL,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT charge_attribution_pkey
        PRIMARY KEY (tenant_id, fee_item),

    CONSTRAINT charge_attribution_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(timezone_name) <> ''
        ),

    CONSTRAINT charge_attribution_form_closed
        CHECK (form IN ('SOURCE_OCCURRED', 'CHARGE_CONFIRMED')),

    CONSTRAINT charge_attribution_cutoff_minute
        CHECK (cutoff_minute BETWEEN 0 AND 1439)
);
