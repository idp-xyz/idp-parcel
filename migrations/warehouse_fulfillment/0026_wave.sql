-- 波次分配与拣货（UC-WF-013）。分配策略不种任何行：没采用时放波次答未配置。
-- 参考策略随产品内置：跟踪效期的先到期先出，其余先进先出；拣选库区先于存储。

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_status;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'SPLIT',
        'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED'
    ));

CREATE TABLE warehouse_fulfillment.allocation_strategy (
    tenant_id     text        NOT NULL,
    revision      integer     NOT NULL,
    operator_id   text        NOT NULL,
    registered_at timestamptz NOT NULL,

    CONSTRAINT allocation_strategy_pkey PRIMARY KEY (tenant_id, revision),
    CONSTRAINT allocation_strategy_revision CHECK (revision >= 1),
    CONSTRAINT allocation_strategy_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(operator_id) <> '')
);

CREATE TABLE warehouse_fulfillment.wave (
    tenant_id     text        NOT NULL,
    wave_id       text        NOT NULL,
    warehouse_code text       NOT NULL,
    operator_id   text        NOT NULL,
    prefetch      boolean     NOT NULL,
    released_at   timestamptz NOT NULL,

    CONSTRAINT wave_pkey PRIMARY KEY (tenant_id, wave_id),
    CONSTRAINT wave_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(wave_id) <> '' AND btrim(warehouse_code) <> '' AND btrim(operator_id) <> '')
);

CREATE TABLE warehouse_fulfillment.wave_request (
    tenant_id   text NOT NULL,
    wave_id     text NOT NULL,
    request_id  text NOT NULL,

    CONSTRAINT wave_request_pkey PRIMARY KEY (tenant_id, wave_id, request_id),
    CONSTRAINT wave_request_wave_fkey
        FOREIGN KEY (tenant_id, wave_id)
        REFERENCES warehouse_fulfillment.wave (tenant_id, wave_id)
);

CREATE TABLE warehouse_fulfillment.pick_task (
    tenant_id        text   NOT NULL,
    task_id          text   NOT NULL,
    wave_id          text   NOT NULL,
    request_id       text   NOT NULL,
    location_code    text   NOT NULL,
    goods_code       text   NOT NULL,
    unit_name        text   NOT NULL,
    stock_state      text   NOT NULL,
    inbound_batch_id text   NOT NULL,
    quantity         bigint NOT NULL,
    picked           bigint NOT NULL,
    status           text   NOT NULL,
    fact_id          text,

    CONSTRAINT pick_task_pkey PRIMARY KEY (tenant_id, task_id),
    CONSTRAINT pick_task_quantity CHECK (quantity > 0 AND picked >= 0 AND picked <= quantity),
    CONSTRAINT pick_task_status CHECK (status IN ('OPEN', 'DONE', 'SHORT'))
);

CREATE TABLE warehouse_fulfillment.pick_difference (
    tenant_id      text   NOT NULL,
    difference_id  text   NOT NULL,
    task_id        text   NOT NULL,
    fact_id        text   NOT NULL,
    quantity       bigint NOT NULL,
    status         text   NOT NULL,

    CONSTRAINT pick_difference_pkey PRIMARY KEY (tenant_id, difference_id),
    CONSTRAINT pick_difference_quantity CHECK (quantity > 0),
    CONSTRAINT pick_difference_status CHECK (status = 'PENDING')
);

CREATE TABLE warehouse_fulfillment.outbound_check (
    tenant_id   text        NOT NULL,
    fact_id     text        NOT NULL,
    request_id  text        NOT NULL,
    observed    bigint      NOT NULL,
    matched     boolean     NOT NULL,
    checked_at  timestamptz NOT NULL,

    CONSTRAINT outbound_check_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT outbound_check_observed CHECK (observed >= 0)
);

CREATE TABLE warehouse_fulfillment.outbound_pack (
    tenant_id  text        NOT NULL,
    fact_id    text        NOT NULL,
    request_id text        NOT NULL,
    packed_at  timestamptz NOT NULL,

    CONSTRAINT outbound_pack_pkey PRIMARY KEY (tenant_id, fact_id)
);

CREATE TABLE warehouse_fulfillment.outbound_parcel (
    tenant_id           text   NOT NULL,
    request_id          text   NOT NULL,
    parcel_sequence     integer NOT NULL,
    packaging_material  text   NOT NULL,
    weight_value        text   NOT NULL,
    weight_unit         text   NOT NULL,
    length_value        text   NOT NULL,
    width_value         text   NOT NULL,
    height_value        text   NOT NULL,
    dimension_unit      text   NOT NULL,

    CONSTRAINT outbound_parcel_pkey PRIMARY KEY (tenant_id, request_id, parcel_sequence),
    CONSTRAINT outbound_parcel_sequence CHECK (parcel_sequence >= 1),
    CONSTRAINT outbound_parcel_not_blank CHECK (
        btrim(packaging_material) <> '' AND btrim(weight_value) <> '' AND btrim(weight_unit) <> ''
        AND btrim(length_value) <> '' AND btrim(width_value) <> '' AND btrim(height_value) <> ''
        AND btrim(dimension_unit) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.outbound_parcel_line (
    tenant_id       text    NOT NULL,
    request_id      text    NOT NULL,
    parcel_sequence integer NOT NULL,
    line_index      integer NOT NULL,
    goods_code      text    NOT NULL,
    unit_name       text    NOT NULL,
    quantity        bigint  NOT NULL,

    CONSTRAINT outbound_parcel_line_pkey PRIMARY KEY (tenant_id, request_id, parcel_sequence, line_index),
    CONSTRAINT outbound_parcel_line_quantity CHECK (quantity > 0 AND line_index >= 0),
    CONSTRAINT outbound_parcel_line_parcel_fkey
        FOREIGN KEY (tenant_id, request_id, parcel_sequence)
        REFERENCES warehouse_fulfillment.outbound_parcel (tenant_id, request_id, parcel_sequence)
);
