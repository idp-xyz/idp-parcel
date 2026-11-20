-- 盘点（UC-WF-010）。静盘：开始时加作业锁定，完成时只比出差额，不改库存账。
-- 本迁移只建结构、不种任何行。

CREATE TABLE warehouse_fulfillment.stock_count (
    tenant_id        text        NOT NULL,
    count_id         text        NOT NULL,
    decision_id      text        NOT NULL,
    warehouse_code   text        NOT NULL,
    owner_account_id text,
    note             text,
    operator_id      text        NOT NULL,
    created_at       timestamptz NOT NULL,
    status           text        NOT NULL,
    started_by       text,
    started_at       timestamptz,
    completed_by     text,
    completed_at     timestamptz,
    voided_by        text,
    voided_at        timestamptz,
    void_reason      text,

    CONSTRAINT stock_count_pkey PRIMARY KEY (tenant_id, count_id),
    CONSTRAINT stock_count_decision UNIQUE (tenant_id, decision_id),
    CONSTRAINT stock_count_status CHECK (status IN ('PENDING', 'COUNTING', 'COUNTED', 'VOID')),
    CONSTRAINT stock_count_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(count_id) <> '' AND btrim(decision_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(operator_id) <> ''
        AND (owner_account_id IS NULL OR btrim(owner_account_id) <> '')
        AND (note IS NULL OR btrim(note) <> '')
        AND (void_reason IS NULL OR btrim(void_reason) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.stock_count_location (
    tenant_id     text NOT NULL,
    count_id      text NOT NULL,
    location_code text NOT NULL,

    CONSTRAINT stock_count_location_pkey PRIMARY KEY (tenant_id, count_id, location_code),
    CONSTRAINT stock_count_location_count_fkey
        FOREIGN KEY (tenant_id, count_id) REFERENCES warehouse_fulfillment.stock_count (tenant_id, count_id),
    CONSTRAINT stock_count_location_not_blank CHECK (btrim(location_code) <> '')
);

CREATE TABLE warehouse_fulfillment.stock_count_goods (
    tenant_id  text NOT NULL,
    count_id   text NOT NULL,
    goods_code text NOT NULL,

    CONSTRAINT stock_count_goods_pkey PRIMARY KEY (tenant_id, count_id, goods_code),
    CONSTRAINT stock_count_goods_count_fkey
        FOREIGN KEY (tenant_id, count_id) REFERENCES warehouse_fulfillment.stock_count (tenant_id, count_id)
);

CREATE TABLE warehouse_fulfillment.stock_count_lock (
    tenant_id        text   NOT NULL,
    count_id         text   NOT NULL,
    lock_index       integer NOT NULL,
    owner_account_id text   NOT NULL,
    warehouse_code   text   NOT NULL,
    location_code    text   NOT NULL,
    goods_code       text   NOT NULL,
    unit_name        text   NOT NULL,
    stock_state      text   NOT NULL,
    label_alias      text,
    inbound_batch_id text   NOT NULL,
    batch_number     text,
    expiry_date      date,
    quantity         bigint NOT NULL,

    CONSTRAINT stock_count_lock_pkey PRIMARY KEY (tenant_id, count_id, lock_index),
    CONSTRAINT stock_count_lock_quantity CHECK (quantity > 0 AND lock_index >= 0)
);

CREATE TABLE warehouse_fulfillment.count_entry (
    tenant_id       text        NOT NULL,
    fact_id         text        NOT NULL,
    count_id        text        NOT NULL,
    location_code   text        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    device_sequence bigint      NOT NULL,
    device_id       text        NOT NULL,
    operator_id     text        NOT NULL,
    received_at     timestamptz NOT NULL,

    CONSTRAINT count_entry_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT count_entry_count_fkey
        FOREIGN KEY (tenant_id, count_id) REFERENCES warehouse_fulfillment.stock_count (tenant_id, count_id)
);

CREATE TABLE warehouse_fulfillment.count_entry_line (
    tenant_id        text   NOT NULL,
    fact_id          text   NOT NULL,
    line_index       integer NOT NULL,
    owner_account_id text,
    goods_code       text,
    unit_name        text,
    stock_state      text,
    label_alias      text,
    batch_number     text,
    expiry_date      date,
    quantity         bigint NOT NULL,
    observed         text,

    CONSTRAINT count_entry_line_pkey PRIMARY KEY (tenant_id, fact_id, line_index),
    CONSTRAINT count_entry_line_entry_fkey
        FOREIGN KEY (tenant_id, fact_id) REFERENCES warehouse_fulfillment.count_entry (tenant_id, fact_id),
    CONSTRAINT count_entry_line_quantity CHECK (quantity >= 0 AND line_index >= 0),
    CONSTRAINT count_entry_line_shape CHECK (
        (observed IS NOT NULL AND btrim(observed) <> '' AND goods_code IS NULL AND owner_account_id IS NULL)
        OR (
            observed IS NULL AND goods_code IS NOT NULL AND unit_name IS NOT NULL
            AND stock_state IS NOT NULL AND owner_account_id IS NOT NULL
        )
    )
);

CREATE TABLE warehouse_fulfillment.count_difference (
    tenant_id        text   NOT NULL,
    difference_id    text   NOT NULL,
    count_id         text   NOT NULL,
    owner_account_id text   NOT NULL,
    warehouse_code   text   NOT NULL,
    location_code    text   NOT NULL,
    goods_code       text   NOT NULL,
    unit_name        text   NOT NULL,
    stock_state      text   NOT NULL,
    label_alias      text,
    batch_number     text,
    expiry_date      date,
    book_quantity    bigint NOT NULL,
    counted_quantity bigint NOT NULL,
    status           text   NOT NULL,

    CONSTRAINT count_difference_pkey PRIMARY KEY (tenant_id, difference_id),
    CONSTRAINT count_difference_status CHECK (status = 'PENDING'),
    CONSTRAINT count_difference_gap CHECK (book_quantity >= 0 AND counted_quantity >= 0 AND book_quantity <> counted_quantity)
);
