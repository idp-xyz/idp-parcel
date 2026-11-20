-- 库内作业事实、库存差异与次品转良品（UC-WF-007）。
-- 整箱移动允许一箱先 -1 再 +1，原因加上 INTERNAL_MOVE。收货仍只写 +1。
-- 本迁移只建结构、不种任何行。

ALTER TABLE warehouse_fulfillment.carton_movement
    DROP CONSTRAINT carton_movement_reason;

ALTER TABLE warehouse_fulfillment.carton_movement
    ADD CONSTRAINT carton_movement_reason
        CHECK (reason IN ('RECEIPT', 'INTERNAL_MOVE'));

ALTER TABLE warehouse_fulfillment.carton_movement_line
    DROP CONSTRAINT carton_movement_line_delta;

ALTER TABLE warehouse_fulfillment.carton_movement_line
    ADD CONSTRAINT carton_movement_line_delta
        CHECK (quantity_delta = 1 OR quantity_delta = -1);

CREATE TABLE warehouse_fulfillment.internal_job (
    tenant_id        text        NOT NULL,
    fact_id          text        NOT NULL,
    kind             text        NOT NULL,
    warehouse_code   text        NOT NULL,
    device_id        text        NOT NULL,
    operator_id      text        NOT NULL,
    occurred_at      timestamptz NOT NULL,
    device_sequence  bigint      NOT NULL,
    received_at      timestamptz NOT NULL,

    CONSTRAINT internal_job_pkey
        PRIMARY KEY (tenant_id, fact_id),

    CONSTRAINT internal_job_kind
        CHECK (kind IN ('INTERNAL_MOVE', 'FORM_CONVERSION', 'CONDITION_CONVERSION', 'SHORTAGE')),

    CONSTRAINT internal_job_sequence
        CHECK (device_sequence >= 1),

    CONSTRAINT internal_job_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(device_id) <> ''
            AND btrim(operator_id) <> ''
        )
);

CREATE TABLE warehouse_fulfillment.stock_discrepancy (
    tenant_id         text    NOT NULL,
    discrepancy_id    text    NOT NULL,
    fact_id           text    NOT NULL,
    line_position     integer NOT NULL,
    owner_account_id  text    NOT NULL,
    warehouse_code    text    NOT NULL,
    location_code     text,
    goods_code        text,
    unit_name         text,
    stock_state       text,
    label_alias       text,
    batch_number      text,
    expiry_date       date,
    inbound_batch_id  text,
    source_location   text,
    carton_number     text,
    note              text,
    quantity          bigint  NOT NULL,
    short_on_book     bigint  NOT NULL,
    held_quantity     bigint  NOT NULL,
    carton_missing    boolean NOT NULL,
    status            text    NOT NULL,

    CONSTRAINT stock_discrepancy_pkey
        PRIMARY KEY (tenant_id, discrepancy_id),

    CONSTRAINT stock_discrepancy_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.internal_job (tenant_id, fact_id),

    CONSTRAINT stock_discrepancy_status
        CHECK (status = 'PENDING'),

    CONSTRAINT stock_discrepancy_quantity
        CHECK (quantity > 0 AND short_on_book >= 0 AND held_quantity >= 0 AND line_position >= 0),

    CONSTRAINT stock_discrepancy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(discrepancy_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND (location_code IS NULL OR btrim(location_code) <> '')
            AND (goods_code IS NULL OR btrim(goods_code) <> '')
            AND (unit_name IS NULL OR btrim(unit_name) <> '')
            AND (stock_state IS NULL OR btrim(stock_state) <> '')
            AND (label_alias IS NULL OR btrim(label_alias) <> '')
            AND (batch_number IS NULL OR btrim(batch_number) <> '')
            AND (inbound_batch_id IS NULL OR btrim(inbound_batch_id) <> '')
            AND (source_location IS NULL OR btrim(source_location) <> '')
            AND (carton_number IS NULL OR btrim(carton_number) <> '')
        )
);

CREATE TABLE warehouse_fulfillment.internal_job_line (
    tenant_id                text    NOT NULL,
    fact_id                  text    NOT NULL,
    position                 integer NOT NULL,
    owner_account_id         text    NOT NULL,
    goods_code               text,
    unit_name                text,
    stock_state              text,
    label_alias              text,
    batch_number             text,
    expiry_date              date,
    inbound_batch_id         text,
    source_location          text,
    target_location          text,
    quantity                 bigint  NOT NULL,
    to_unit                  text,
    to_quantity              bigint,
    to_state                 text,
    carton_number            text,
    note                     text,
    posted_quantity          bigint  NOT NULL,
    posted_inbound_quantity  bigint  NOT NULL,
    short_on_book            bigint  NOT NULL,
    held_quantity            bigint  NOT NULL,
    carton_missing           boolean NOT NULL,
    movement_id              text,
    discrepancy_id           text,

    CONSTRAINT internal_job_line_pkey
        PRIMARY KEY (tenant_id, fact_id, position),

    CONSTRAINT internal_job_line_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.internal_job (tenant_id, fact_id),

    CONSTRAINT internal_job_line_position
        CHECK (position >= 0 AND posted_quantity >= 0 AND posted_inbound_quantity >= 0 AND short_on_book >= 0 AND held_quantity >= 0),

    CONSTRAINT internal_job_line_quantity
        CHECK (
            (carton_number IS NOT NULL AND goods_code IS NULL AND quantity >= 0)
            OR (carton_number IS NULL AND goods_code IS NOT NULL AND quantity > 0)
        ),

    CONSTRAINT internal_job_line_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND (goods_code IS NULL OR btrim(goods_code) <> '')
            AND (unit_name IS NULL OR btrim(unit_name) <> '')
            AND (stock_state IS NULL OR btrim(stock_state) <> '')
            AND (label_alias IS NULL OR btrim(label_alias) <> '')
            AND (batch_number IS NULL OR btrim(batch_number) <> '')
            AND (inbound_batch_id IS NULL OR btrim(inbound_batch_id) <> '')
            AND (source_location IS NULL OR btrim(source_location) <> '')
            AND (target_location IS NULL OR btrim(target_location) <> '')
            AND (to_unit IS NULL OR btrim(to_unit) <> '')
            AND (to_state IS NULL OR btrim(to_state) <> '')
            AND (carton_number IS NULL OR btrim(carton_number) <> '')
            AND (movement_id IS NULL OR btrim(movement_id) <> '')
            AND (discrepancy_id IS NULL OR btrim(discrepancy_id) <> '')
            AND (to_quantity IS NULL OR to_quantity > 0)
        )
);

CREATE TABLE warehouse_fulfillment.internal_job_line_batch (
    tenant_id         text    NOT NULL,
    fact_id           text    NOT NULL,
    position          integer NOT NULL,
    batch_position    integer NOT NULL,
    inbound_batch_id  text    NOT NULL,

    CONSTRAINT internal_job_line_batch_pkey
        PRIMARY KEY (tenant_id, fact_id, position, batch_position),

    CONSTRAINT internal_job_line_batch_line_fkey
        FOREIGN KEY (tenant_id, fact_id, position)
        REFERENCES warehouse_fulfillment.internal_job_line (tenant_id, fact_id, position),

    CONSTRAINT internal_job_line_batch_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(inbound_batch_id) <> '' AND batch_position >= 0)
);

CREATE INDEX internal_job_received_idx
    ON warehouse_fulfillment.internal_job (tenant_id, received_at DESC, fact_id);

CREATE TABLE warehouse_fulfillment.condition_restore (
    tenant_id         text        NOT NULL,
    decision_id       text        NOT NULL,
    operator_id       text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    received_at       timestamptz NOT NULL,
    warehouse_code    text        NOT NULL,
    owner_account_id  text        NOT NULL,
    location_code     text        NOT NULL,
    goods_code        text        NOT NULL,
    unit_name         text        NOT NULL,
    label_alias       text,
    batch_number      text,
    expiry_date       date,
    inbound_batch_id  text,
    quantity          bigint      NOT NULL,
    reason            text        NOT NULL,
    movement_id       text        NOT NULL,

    CONSTRAINT condition_restore_pkey
        PRIMARY KEY (tenant_id, decision_id),

    CONSTRAINT condition_restore_movement_fkey
        FOREIGN KEY (tenant_id, movement_id)
        REFERENCES warehouse_fulfillment.stock_movement (tenant_id, movement_id),

    CONSTRAINT condition_restore_quantity
        CHECK (quantity > 0),

    CONSTRAINT condition_restore_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(operator_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(location_code) <> ''
            AND btrim(goods_code) <> ''
            AND btrim(unit_name) <> ''
            AND btrim(reason) <> ''
            AND btrim(movement_id) <> ''
            AND (label_alias IS NULL OR btrim(label_alias) <> '')
            AND (batch_number IS NULL OR btrim(batch_number) <> '')
            AND (inbound_batch_id IS NULL OR btrim(inbound_batch_id) <> '')
        )
);
