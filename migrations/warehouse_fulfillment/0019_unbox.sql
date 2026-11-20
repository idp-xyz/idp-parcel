-- 开箱（UC-WF-008）。整箱账的原因加上 UNBOXING：箱减一，不新起整箱批次。
-- 商品按实点入商品账，入库时间沿用该箱。实点与箱单的差额记在开箱事实下。
-- 本迁移只建结构、不种任何行。

ALTER TABLE warehouse_fulfillment.carton_movement
    DROP CONSTRAINT carton_movement_reason;

ALTER TABLE warehouse_fulfillment.carton_movement
    ADD CONSTRAINT carton_movement_reason
        CHECK (reason IN ('RECEIPT', 'INTERNAL_MOVE', 'UNBOXING'));

CREATE TABLE warehouse_fulfillment.unbox_fact (
    tenant_id        text        NOT NULL,
    fact_id          text        NOT NULL,
    owner_account_id text        NOT NULL,
    warehouse_code   text        NOT NULL,
    carton_number    text        NOT NULL,
    device_id        text        NOT NULL,
    operator_id      text        NOT NULL,
    occurred_at      timestamptz NOT NULL,
    device_sequence  bigint      NOT NULL,
    received_at      timestamptz NOT NULL,
    movement_id      text,

    CONSTRAINT unbox_fact_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT unbox_fact_sequence CHECK (device_sequence >= 1),
    CONSTRAINT unbox_fact_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(carton_number) <> ''
        AND btrim(device_id) <> '' AND btrim(operator_id) <> ''
        AND (movement_id IS NULL OR btrim(movement_id) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.unbox_line (
    tenant_id      text    NOT NULL,
    fact_id        text    NOT NULL,
    position       integer NOT NULL,
    observed_text  text,
    goods_code     text,
    unit_name      text,
    stock_state    text,
    label_alias    text,
    batch_number   text,
    expiry_date    date,
    location_code  text,
    quantity       bigint  NOT NULL,

    CONSTRAINT unbox_line_pkey PRIMARY KEY (tenant_id, fact_id, position),
    CONSTRAINT unbox_line_fact_fkey
        FOREIGN KEY (tenant_id, fact_id) REFERENCES warehouse_fulfillment.unbox_fact (tenant_id, fact_id),
    CONSTRAINT unbox_line_quantity CHECK (quantity > 0 AND position >= 0),
    CONSTRAINT unbox_line_subject CHECK (
        (goods_code IS NULL AND observed_text IS NOT NULL AND btrim(observed_text) <> '')
        OR (goods_code IS NOT NULL AND btrim(goods_code) <> '' AND unit_name IS NOT NULL AND stock_state IS NOT NULL AND location_code IS NOT NULL)
    )
);

CREATE TABLE warehouse_fulfillment.unbox_difference (
    tenant_id     text    NOT NULL,
    fact_id       text    NOT NULL,
    position      integer NOT NULL,
    kind          text    NOT NULL,
    quantity      bigint  NOT NULL,
    goods_code    text,
    observed_text text,

    CONSTRAINT unbox_difference_pkey PRIMARY KEY (tenant_id, fact_id, position),
    CONSTRAINT unbox_difference_fact_fkey
        FOREIGN KEY (tenant_id, fact_id) REFERENCES warehouse_fulfillment.unbox_fact (tenant_id, fact_id),
    CONSTRAINT unbox_difference_kind CHECK (kind IN ('SHORT', 'OVERAGE', 'DAMAGE', 'WRONG_GOODS')),
    CONSTRAINT unbox_difference_quantity CHECK (quantity > 0 AND position >= 0)
);
