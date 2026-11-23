-- 出库确认的更正（UC-WF-015）。一版只更正一次。判不成立先停在待回补，批准后才把消耗调增回待出库。

CREATE TABLE warehouse_fulfillment.outbound_confirmation_correction (
    tenant_id      text        NOT NULL,
    fact_id        text        NOT NULL,
    kind           text        NOT NULL,
    occurred_at    timestamptz,
    operator_id    text        NOT NULL,
    status         text        NOT NULL,
    duty_revision  integer,
    approver_id    text,
    approved_at    timestamptz,

    CONSTRAINT outbound_confirmation_correction_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT outbound_confirmation_correction_kind CHECK (kind IN ('TIME', 'INVALIDATE')),
    CONSTRAINT outbound_confirmation_correction_status CHECK (status IN ('TIME_CORRECTED', 'PENDING', 'POSTED')),
    CONSTRAINT outbound_confirmation_correction_shape CHECK (
        (kind = 'TIME' AND status = 'TIME_CORRECTED' AND occurred_at IS NOT NULL
            AND duty_revision IS NULL AND approver_id IS NULL AND approved_at IS NULL)
        OR (kind = 'INVALIDATE' AND status = 'PENDING' AND occurred_at IS NULL
            AND duty_revision IS NULL AND approver_id IS NULL AND approved_at IS NULL)
        OR (kind = 'INVALIDATE' AND status = 'POSTED' AND occurred_at IS NULL
            AND duty_revision IS NOT NULL AND approver_id IS NOT NULL AND approved_at IS NOT NULL)
    ),
    CONSTRAINT outbound_confirmation_correction_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(operator_id) <> ''
        AND (approver_id IS NULL OR btrim(approver_id) <> '')
    )
);
