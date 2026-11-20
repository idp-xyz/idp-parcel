-- 接受决定信封的入账。来源不是仓配出库的也记一行，装箱记录不动。
-- 仓配出库的接受或拒绝写在对应包裹上。同一决定标识再投认已有结果。

CREATE TABLE warehouse_fulfillment.acceptance_posting (
    tenant_id          text        NOT NULL,
    decision_id        text        NOT NULL,
    source             text        NOT NULL,
    source_request_key text        NOT NULL,
    state              text        NOT NULL,
    posted_at          timestamptz NOT NULL,

    CONSTRAINT acceptance_posting_pkey PRIMARY KEY (tenant_id, decision_id),
    CONSTRAINT acceptance_posting_state CHECK (state IN ('ACCEPTED', 'REJECTED')),
    CONSTRAINT acceptance_posting_not_blank CHECK (
        btrim(decision_id) <> '' AND btrim(source) <> '' AND btrim(source_request_key) <> ''
    )
);

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN commission_state text;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_commission_state CHECK (
        commission_state IS NULL
        OR (commission_state IN ('ACCEPTED', 'REJECTED') AND shipment_request_id IS NOT NULL)
    );
