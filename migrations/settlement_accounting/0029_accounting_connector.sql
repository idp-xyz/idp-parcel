-- 某个对方的一种交换采用规范文书。不预列供应商或财务系统。不登任何租户的行。不设修订。
-- 0028 留给周期费用。

CREATE TABLE settlement_accounting.accounting_connector (
    tenant_id      text        NOT NULL,
    exchange_kind  text        NOT NULL,
    counterparty   text        NOT NULL,
    form           text        NOT NULL,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT accounting_connector_pkey
        PRIMARY KEY (tenant_id, exchange_kind, counterparty),

    CONSTRAINT accounting_connector_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(counterparty) <> ''
        ),

    CONSTRAINT accounting_connector_kind_closed
        CHECK (exchange_kind IN ('SUPPLIER_BILL', 'EXTERNAL_FUNDS')),

    CONSTRAINT accounting_connector_form_closed
        CHECK (form IN ('CANONICAL'))
);
