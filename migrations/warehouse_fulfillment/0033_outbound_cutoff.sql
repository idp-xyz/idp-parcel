-- 出库截单（UC-WF-016）。打包前整份取消并释放还没消耗的预占。已拣出的生成回库任务，货先留在待出库。

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_status;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'SPLIT',
        'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED',
        'CANCELLED', 'AWAITING_RETURN', 'PARTIALLY_CANCELLED'
    ));

ALTER TABLE warehouse_fulfillment.pick_task
    DROP CONSTRAINT pick_task_status;

ALTER TABLE warehouse_fulfillment.pick_task
    ADD CONSTRAINT pick_task_status CHECK (status IN ('OPEN', 'DONE', 'SHORT', 'CANCELLED', 'LATE'));

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN cutoff_state text,
    ADD CONSTRAINT outbound_parcel_cutoff_state CHECK (cutoff_state IS NULL OR cutoff_state = 'CANCELLED');

CREATE TABLE warehouse_fulfillment.outbound_cutoff (
    tenant_id       text        NOT NULL,
    cutoff_key      text        NOT NULL,
    request_id      text        NOT NULL,
    parcel_sequence integer,
    basis           text,
    outcome         text        NOT NULL,
    posted_at       timestamptz NOT NULL,

    CONSTRAINT outbound_cutoff_pkey PRIMARY KEY (tenant_id, cutoff_key),
    CONSTRAINT outbound_cutoff_sequence CHECK (parcel_sequence IS NULL OR parcel_sequence >= 1),
    CONSTRAINT outbound_cutoff_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(cutoff_key) <> '' AND btrim(request_id) <> ''
        AND btrim(outcome) <> '' AND (basis IS NULL OR btrim(basis) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.return_task (
    tenant_id        text   NOT NULL,
    return_id        text   NOT NULL,
    request_id       text   NOT NULL,
    parcel_sequence  integer,
    location_code    text   NOT NULL,
    goods_code       text   NOT NULL,
    unit_name        text   NOT NULL,
    stock_state      text   NOT NULL,
    inbound_batch_id text   NOT NULL,
    quantity         bigint NOT NULL,
    status           text   NOT NULL,

    CONSTRAINT return_task_pkey PRIMARY KEY (tenant_id, return_id),
    CONSTRAINT return_task_quantity CHECK (quantity > 0),
    CONSTRAINT return_task_status CHECK (status = 'OPEN'),
    CONSTRAINT return_task_sequence CHECK (parcel_sequence IS NULL OR parcel_sequence >= 1),
    CONSTRAINT return_task_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(return_id) <> '' AND btrim(request_id) <> ''
        AND btrim(location_code) <> '' AND btrim(goods_code) <> '' AND btrim(unit_name) <> ''
        AND btrim(stock_state) <> '' AND btrim(inbound_batch_id) <> ''
    )
);
