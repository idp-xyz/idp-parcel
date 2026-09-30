-- 一条发生项原因采用「发生项形成」这一触发时点。不预列原因。不登任何租户的行。不设修订。

CREATE TABLE settlement_accounting.buy_evaluation_trigger (
    tenant_id          text        NOT NULL,
    occurrence_reason  text        NOT NULL,
    moment             text        NOT NULL,
    registered_at      timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buy_evaluation_trigger_pkey
        PRIMARY KEY (tenant_id, occurrence_reason),

    CONSTRAINT buy_evaluation_trigger_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(occurrence_reason) <> ''
        ),

    CONSTRAINT buy_evaluation_trigger_moment_closed
        CHECK (moment IN ('OCCURRENCE_FORMED'))
);
