-- 整箱库存（仓配履约 CONTEXT「整箱库存」「库存账与守恒」）。
-- 与商品库存分开：箱不进 stock_movement_line，也不进商品的 inbound_batch。
-- 一箱是一件。余额不存，由 carton_movement_line.quantity_delta 现算。
-- 一笔流水只追加，绝不 UPDATE、绝不 DELETE。
--
-- 在库期间箱号在（租户，货主，仓）内唯一。那是余额为正，不是永久唯一：
-- 约束表达不了「把已入账的行加总再看这一箱还在不在」，写入侧在整箱账自己的事务级咨询锁里判完再插。
-- 这把锁不拿商品库存的货主锁。
--
-- 收货为一箱形成一个入库批次。同一事实的多箱各一个，按入账的箱号区分。
-- 这一段的原因只有收货，数量只有 +1。库区只许收货暂存、存储与待出库，由写入侧核；本表不存库区。
-- 货主原箱号只在仓另编了箱号时留下，并且与入账的箱号不同。
--
-- 收货事实上的整箱行另表存放。商品收货行的形状不动。
-- 收货差异可以逐箱记错货或破损。商品差异的形状照旧。
--
-- 本迁移不对预报、库位的修订表建外键。本迁移只建结构、不种任何行。

CREATE TABLE warehouse_fulfillment.carton_movement (
    tenant_id         text        NOT NULL,
    movement_id       text        NOT NULL,
    owner_account_id  text        NOT NULL,
    warehouse_code    text        NOT NULL,
    reason            text        NOT NULL,
    source_reference  text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    posted_at         timestamptz NOT NULL,

    CONSTRAINT carton_movement_pkey
        PRIMARY KEY (tenant_id, movement_id),

    CONSTRAINT carton_movement_source_key
        UNIQUE (tenant_id, reason, source_reference),

    CONSTRAINT carton_movement_scope_key
        UNIQUE (tenant_id, movement_id, owner_account_id, warehouse_code),

    CONSTRAINT carton_movement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(movement_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(source_reference) <> ''
        ),

    CONSTRAINT carton_movement_reason
        CHECK (reason IN ('RECEIPT'))
);

CREATE TABLE warehouse_fulfillment.carton_inbound_batch (
    tenant_id             text        NOT NULL,
    inbound_batch_id      text        NOT NULL,
    owner_account_id      text        NOT NULL,
    warehouse_code        text        NOT NULL,
    carton_number         text        NOT NULL,
    inbound_at            timestamptz NOT NULL,
    formed_by_movement_id text        NOT NULL,

    CONSTRAINT carton_inbound_batch_pkey
        PRIMARY KEY (tenant_id, inbound_batch_id),

    CONSTRAINT carton_inbound_batch_movement_fkey
        FOREIGN KEY (tenant_id, formed_by_movement_id)
        REFERENCES warehouse_fulfillment.carton_movement (tenant_id, movement_id),

    CONSTRAINT carton_inbound_batch_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(inbound_batch_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(carton_number) <> ''
            AND btrim(formed_by_movement_id) <> ''
        )
);

CREATE TABLE warehouse_fulfillment.carton_movement_line (
    tenant_id            text    NOT NULL,
    movement_id          text    NOT NULL,
    position             integer NOT NULL,
    owner_account_id     text    NOT NULL,
    warehouse_code       text    NOT NULL,
    location_code        text    NOT NULL,
    carton_number        text    NOT NULL,
    owner_carton_number  text,
    inbound_batch_id     text    NOT NULL,
    quantity_delta       bigint  NOT NULL,

    CONSTRAINT carton_movement_line_pkey
        PRIMARY KEY (tenant_id, movement_id, position),

    CONSTRAINT carton_movement_line_movement_fkey
        FOREIGN KEY (tenant_id, movement_id, owner_account_id, warehouse_code)
        REFERENCES warehouse_fulfillment.carton_movement (tenant_id, movement_id, owner_account_id, warehouse_code),

    CONSTRAINT carton_movement_line_batch_fkey
        FOREIGN KEY (tenant_id, inbound_batch_id)
        REFERENCES warehouse_fulfillment.carton_inbound_batch (tenant_id, inbound_batch_id),

    CONSTRAINT carton_movement_line_position
        CHECK (position >= 0),

    -- 一箱是一件。这一段的收货只写 +1。
    CONSTRAINT carton_movement_line_delta
        CHECK (quantity_delta = 1),

    CONSTRAINT carton_movement_line_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(movement_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(location_code) <> ''
            AND btrim(carton_number) <> ''
            AND btrim(inbound_batch_id) <> ''
        ),

    CONSTRAINT carton_movement_line_owner_number
        CHECK (
            owner_carton_number IS NULL
            OR (
                btrim(owner_carton_number) <> ''
                AND owner_carton_number <> carton_number
            )
        )
);

CREATE INDEX carton_movement_line_carton_idx
    ON warehouse_fulfillment.carton_movement_line (
        tenant_id, owner_account_id, warehouse_code, carton_number
    );

CREATE TABLE warehouse_fulfillment.receipt_confirmation_carton_line (
    tenant_id               text    NOT NULL,
    fact_id                 text    NOT NULL,
    line_index              integer NOT NULL,
    carton_number           text    NOT NULL,
    warehouse_carton_number text,
    damaged                 boolean NOT NULL,
    location_code           text    NOT NULL,
    result                  text,
    unposted_reason         text,

    CONSTRAINT receipt_confirmation_carton_line_pkey
        PRIMARY KEY (tenant_id, fact_id, line_index),

    CONSTRAINT receipt_confirmation_carton_line_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.receipt_confirmation (tenant_id, fact_id),

    CONSTRAINT receipt_confirmation_carton_line_not_blank
        CHECK (
            btrim(carton_number) <> ''
            AND btrim(location_code) <> ''
            AND line_index >= 0
        ),

    CONSTRAINT receipt_confirmation_carton_line_warehouse_number
        CHECK (warehouse_carton_number IS NULL OR btrim(warehouse_carton_number) <> ''),

    CONSTRAINT receipt_confirmation_carton_line_result_shape
        CHECK (
            (
                result = 'POSTED'
                AND unposted_reason IS NULL
            )
            OR (
                result IS NULL
                AND unposted_reason = 'WRONG_CARTON'
            )
            OR (
                result IS NULL
                AND unposted_reason = 'CARTON_NUMBER_IN_USE'
            )
            OR (
                result IS NULL
                AND unposted_reason = 'NOT_RECEIVABLE'
            )
        )
);

ALTER TABLE warehouse_fulfillment.receipt_discrepancy
    ADD COLUMN carton_number text;

ALTER TABLE warehouse_fulfillment.receipt_discrepancy
    ADD CONSTRAINT receipt_discrepancy_carton_blank
        CHECK (carton_number IS NULL OR btrim(carton_number) <> '');

ALTER TABLE warehouse_fulfillment.receipt_discrepancy
    DROP CONSTRAINT receipt_discrepancy_subject;

ALTER TABLE warehouse_fulfillment.receipt_discrepancy
    ADD CONSTRAINT receipt_discrepancy_subject
        CHECK (
            (
                kind IN ('OVERAGE', 'DAMAGE')
                AND goods_code IS NOT NULL
                AND btrim(goods_code) <> ''
                AND observed_text IS NULL
                AND carton_number IS NULL
            )
            OR (
                kind = 'DAMAGE'
                AND carton_number IS NOT NULL
                AND btrim(carton_number) <> ''
                AND goods_code IS NULL
                AND observed_text IS NULL
            )
            OR (
                kind = 'WRONG_GOODS'
                AND (
                    (
                        goods_code IS NOT NULL
                        AND btrim(goods_code) <> ''
                        AND observed_text IS NULL
                        AND carton_number IS NULL
                    )
                    OR (
                        goods_code IS NULL
                        AND observed_text IS NOT NULL
                        AND btrim(observed_text) <> ''
                        AND carton_number IS NULL
                    )
                    OR (
                        goods_code IS NULL
                        AND observed_text IS NULL
                        AND carton_number IS NOT NULL
                        AND btrim(carton_number) <> ''
                    )
                )
            )
        );
