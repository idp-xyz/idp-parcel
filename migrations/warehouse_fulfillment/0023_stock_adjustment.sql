-- 库存调整（UC-WF-011）。提交不改账，批准才入账。
-- 库内作业差异的批准与驳回记在原表上。本迁移不种任何行。

ALTER TABLE warehouse_fulfillment.stock_discrepancy
    DROP CONSTRAINT stock_discrepancy_status;

ALTER TABLE warehouse_fulfillment.stock_discrepancy
    ADD COLUMN approved_by text,
    ADD COLUMN approved_at timestamptz,
    ADD COLUMN approval_note text,
    ADD COLUMN duty_revision integer,
    ADD COLUMN rejected_by text,
    ADD COLUMN rejected_at timestamptz,
    ADD COLUMN reject_reason text;

ALTER TABLE warehouse_fulfillment.stock_discrepancy
    ADD CONSTRAINT stock_discrepancy_status CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    ADD CONSTRAINT stock_discrepancy_approved CHECK (
        status <> 'APPROVED' OR (
            approved_by IS NOT NULL AND approved_at IS NOT NULL AND duty_revision IS NOT NULL
            AND rejected_by IS NULL AND rejected_at IS NULL
        )
    ),
    ADD CONSTRAINT stock_discrepancy_rejected CHECK (
        status <> 'REJECTED' OR (
            rejected_by IS NOT NULL AND rejected_at IS NOT NULL AND reject_reason IS NOT NULL
            AND approved_by IS NULL AND approved_at IS NULL
        )
    );

CREATE TABLE warehouse_fulfillment.stock_adjustment (
    tenant_id        text        NOT NULL,
    adjustment_id    text        NOT NULL,
    owner_account_id text        NOT NULL,
    warehouse_code   text        NOT NULL,
    reason           text        NOT NULL,
    operator_id      text        NOT NULL,
    submitted_at     timestamptz NOT NULL,
    status           text        NOT NULL,
    approved_by      text,
    approved_at      timestamptz,
    approval_note    text,
    duty_revision    integer,
    rejected_by      text,
    rejected_at      timestamptz,
    reject_reason    text,

    CONSTRAINT stock_adjustment_pkey PRIMARY KEY (tenant_id, adjustment_id),
    CONSTRAINT stock_adjustment_status CHECK (status IN ('PENDING', 'POSTED', 'REJECTED')),
    CONSTRAINT stock_adjustment_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(adjustment_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(reason) <> '' AND btrim(operator_id) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.stock_adjustment_line (
    tenant_id        text    NOT NULL,
    adjustment_id    text    NOT NULL,
    line_index       integer NOT NULL,
    kind             text    NOT NULL,
    location_code    text    NOT NULL,
    goods_code       text    NOT NULL,
    unit_name        text    NOT NULL,
    stock_state      text    NOT NULL,
    label_alias      text,
    batch_number     text,
    expiry_date      date,
    inbound_batch_id text,
    quantity         bigint  NOT NULL,
    basis            text,

    CONSTRAINT stock_adjustment_line_pkey PRIMARY KEY (tenant_id, adjustment_id, line_index),
    CONSTRAINT stock_adjustment_line_adjustment_fkey
        FOREIGN KEY (tenant_id, adjustment_id)
        REFERENCES warehouse_fulfillment.stock_adjustment (tenant_id, adjustment_id),
    CONSTRAINT stock_adjustment_line_kind CHECK (kind IN ('INCREASE', 'DECREASE', 'DESTRUCTION')),
    CONSTRAINT stock_adjustment_line_quantity CHECK (quantity > 0 AND line_index >= 0),
    CONSTRAINT stock_adjustment_line_basis CHECK (
        (kind = 'DESTRUCTION' AND basis IS NOT NULL AND btrim(basis) <> '')
        OR (kind <> 'DESTRUCTION' AND basis IS NULL)
    )
);
