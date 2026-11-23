-- 仓储计费采用与这一路的 SELL 请求（UC-SA-002）。键是货主客户账户与仓。不种任何租户的行。

CREATE TABLE settlement_accounting.warehouse_charge_adoption (
    tenant_id         text        NOT NULL,
    owner_account_id  text        NOT NULL,
    warehouse_code    text        NOT NULL,
    revision          integer     NOT NULL,
    effective_at      timestamptz NOT NULL,
    account_id        text        NOT NULL,
    pricing_scope     text        NOT NULL,
    basis             text        NOT NULL,
    registrant        text        NOT NULL,

    CONSTRAINT warehouse_charge_adoption_pkey
        PRIMARY KEY (tenant_id, owner_account_id, warehouse_code, revision),
    CONSTRAINT warehouse_charge_adoption_revision CHECK (revision >= 1),
    CONSTRAINT warehouse_charge_adoption_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(owner_account_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(account_id) <> '' AND btrim(pricing_scope) <> '' AND btrim(basis) <> ''
        AND btrim(registrant) <> ''
    )
);

CREATE TABLE settlement_accounting.warehouse_charge_request (
    tenant_id      text    NOT NULL,
    occurrence_id  text    NOT NULL,
    version        integer NOT NULL,
    status         text    NOT NULL,
    charge_id      text,
    evaluation_id  text,

    CONSTRAINT warehouse_charge_request_pkey PRIMARY KEY (tenant_id, occurrence_id, version),
    CONSTRAINT warehouse_charge_request_version CHECK (version >= 1),
    CONSTRAINT warehouse_charge_request_status CHECK (status IN (
        'FORMED', 'CONFIRMED', 'ZERO', 'PENDING', 'NOT_CONFIGURED', 'AWAITING_PERSON', 'SUPERSEDED'
    )),
    CONSTRAINT warehouse_charge_request_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(occurrence_id) <> '')
);
