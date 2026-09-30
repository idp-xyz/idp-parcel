-- 租户采用了确认或截单。不存钟点，不存账期。不登任何租户的行。不设修订。

CREATE TABLE settlement_accounting.settlement_moment (
    tenant_id     text        NOT NULL,
    moment        text        NOT NULL,
    registered_at timestamptz NOT NULL,
    inserted_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT settlement_moment_pkey
        PRIMARY KEY (tenant_id, moment),

    CONSTRAINT settlement_moment_scope_not_blank
        CHECK (btrim(tenant_id) <> ''),

    CONSTRAINT settlement_moment_closed
        CHECK (moment IN ('CONFIRM', 'CUT_OFF'))
);
