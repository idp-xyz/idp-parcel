-- SELL 评价的触发时点。与 BUY 评价触发册分开，不共用请求身份。不预列原因。不登任何租户的行。

CREATE TABLE settlement_accounting.sell_evaluation_trigger (
    tenant_id          text        NOT NULL,
    occurrence_reason  text        NOT NULL,
    moment             text        NOT NULL,
    registered_at      timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sell_evaluation_trigger_pkey
        PRIMARY KEY (tenant_id, occurrence_reason),

    CONSTRAINT sell_evaluation_trigger_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(occurrence_reason) <> ''
        ),

    CONSTRAINT sell_evaluation_trigger_moment_closed
        CHECK (moment IN ('OCCURRENCE_FORMED'))
);
