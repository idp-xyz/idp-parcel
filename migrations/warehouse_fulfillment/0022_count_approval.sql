-- 盘点差额的批准与驳回，以及批准职责规则（UC-WF-011）。
-- 规则不带默认：没登记时批准答未配置。本迁移不种任何行。

ALTER TABLE warehouse_fulfillment.count_difference
    DROP CONSTRAINT count_difference_status;

ALTER TABLE warehouse_fulfillment.count_difference
    ADD COLUMN approved_by text,
    ADD COLUMN approved_at timestamptz,
    ADD COLUMN approval_note text,
    ADD COLUMN duty_revision integer,
    ADD COLUMN rejected_by text,
    ADD COLUMN rejected_at timestamptz,
    ADD COLUMN reject_reason text;

ALTER TABLE warehouse_fulfillment.count_difference
    ADD CONSTRAINT count_difference_status CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    ADD CONSTRAINT count_difference_approved CHECK (
        status <> 'APPROVED' OR (
            approved_by IS NOT NULL AND approved_at IS NOT NULL AND duty_revision IS NOT NULL
            AND rejected_by IS NULL AND rejected_at IS NULL
        )
    ),
    ADD CONSTRAINT count_difference_rejected CHECK (
        status <> 'REJECTED' OR (
            rejected_by IS NOT NULL AND rejected_at IS NOT NULL AND reject_reason IS NOT NULL
            AND approved_by IS NULL AND approved_at IS NULL
        )
    );

CREATE TABLE warehouse_fulfillment.approval_duty (
    tenant_id           text        NOT NULL,
    revision            integer     NOT NULL,
    distinct_subjects   boolean     NOT NULL,
    operator_id         text        NOT NULL,
    registered_at       timestamptz NOT NULL,

    CONSTRAINT approval_duty_pkey PRIMARY KEY (tenant_id, revision),
    CONSTRAINT approval_duty_revision CHECK (revision >= 1),
    CONSTRAINT approval_duty_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(operator_id) <> '')
);
