-- 库存账（仓配履约 CONTEXT「库存账」「库存流水」「入库批次」与「库存账与守恒」）。
-- 余额不存，由 stock_movement_line 的 quantity_delta 现算。一笔流水只追加，绝不 UPDATE、绝不 DELETE。
--
-- 判重位置是（tenant_id, reason, source_reference），至多一笔。
-- 「取走之后余额不为负」不是约束能表达的：它要按格把已入账的行加总，再看这一笔的净取走。
-- 写入侧在（租户，货主，仓）的事务级咨询锁里判完再插。绕过写入侧的直插不在这把锁的范围内。
--
-- 本迁移不对仓、库区、库位、商品的修订表建外键：那些是只追加的修订表，没有一行可以指作「当前」。
-- 行上的货主与仓靠指向流水头的复合外键与头保持一致，不靠指向修订表。
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。
--
-- 可空文本（批号、贴标别名）在 CHECK 里写明「是 NULL」或「非空白」，避开 SQL 三值逻辑把 NULL 判成通过。
-- 效期是 date，只到日。

CREATE TABLE warehouse_fulfillment.stock_movement (
    tenant_id         text        NOT NULL,
    movement_id       text        NOT NULL,
    owner_account_id  text        NOT NULL,
    warehouse_code    text        NOT NULL,
    reason            text        NOT NULL,
    source_reference  text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    posted_at         timestamptz NOT NULL,

    CONSTRAINT stock_movement_pkey
        PRIMARY KEY (tenant_id, movement_id),

    CONSTRAINT stock_movement_source_key
        UNIQUE (tenant_id, reason, source_reference),

    -- 行表的复合外键指这里，使行上冗余的货主与仓不可能与流水头不一致。
    CONSTRAINT stock_movement_scope_key
        UNIQUE (tenant_id, movement_id, owner_account_id, warehouse_code),

    CONSTRAINT stock_movement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(movement_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(source_reference) <> ''
        ),

    CONSTRAINT stock_movement_reason
        CHECK (reason IN (
            'RECEIPT',
            'RETURN_RECEIPT',
            'TAKE_ON_IMPORT',
            'INTERNAL_MOVE',
            'UNIT_CONVERSION',
            'STATE_CHANGE',
            'RELABEL',
            'PROCESSING',
            'DISASSEMBLY',
            'UNBOXING',
            'OUTBOUND_CONFIRMATION',
            'COUNT_GAIN',
            'COUNT_LOSS',
            'ADJUSTMENT_INCREASE',
            'ADJUSTMENT_DECREASE',
            'DESTRUCTION'
        ))
);

CREATE TABLE warehouse_fulfillment.inbound_batch (
    tenant_id              text        NOT NULL,
    inbound_batch_id       text        NOT NULL,
    owner_account_id       text        NOT NULL,
    warehouse_code         text        NOT NULL,
    goods_code             text        NOT NULL,
    batch_number           text,
    expiry_date            date,
    inbound_at             timestamptz NOT NULL,
    formed_by_movement_id  text        NOT NULL,

    CONSTRAINT inbound_batch_pkey
        PRIMARY KEY (tenant_id, inbound_batch_id),

    CONSTRAINT inbound_batch_movement_fkey
        FOREIGN KEY (tenant_id, formed_by_movement_id)
        REFERENCES warehouse_fulfillment.stock_movement (tenant_id, movement_id),

    CONSTRAINT inbound_batch_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(inbound_batch_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(goods_code) <> ''
            AND btrim(formed_by_movement_id) <> ''
        ),

    CONSTRAINT inbound_batch_number_blank
        CHECK (batch_number IS NULL OR btrim(batch_number) <> '')
);

CREATE TABLE warehouse_fulfillment.stock_movement_line (
    tenant_id             text        NOT NULL,
    movement_id           text        NOT NULL,
    position              integer     NOT NULL,
    owner_account_id      text        NOT NULL,
    warehouse_code        text        NOT NULL,
    location_code         text        NOT NULL,
    goods_code            text        NOT NULL,
    unit_name             text        NOT NULL,
    stock_state           text        NOT NULL,
    label_alias           text,
    inbound_batch_id      text        NOT NULL,
    batch_number          text,
    expiry_date           date,
    quantity_delta        bigint      NOT NULL,
    declared_inbound_at   timestamptz,
    formed_inbound_batch  boolean     NOT NULL,

    CONSTRAINT stock_movement_line_pkey
        PRIMARY KEY (tenant_id, movement_id, position),

    CONSTRAINT stock_movement_line_movement_fkey
        FOREIGN KEY (tenant_id, movement_id, owner_account_id, warehouse_code)
        REFERENCES warehouse_fulfillment.stock_movement (tenant_id, movement_id, owner_account_id, warehouse_code),

    CONSTRAINT stock_movement_line_batch_fkey
        FOREIGN KEY (tenant_id, inbound_batch_id)
        REFERENCES warehouse_fulfillment.inbound_batch (tenant_id, inbound_batch_id),

    CONSTRAINT stock_movement_line_position
        CHECK (position >= 0),

    CONSTRAINT stock_movement_line_delta
        CHECK (quantity_delta <> 0),

    CONSTRAINT stock_movement_line_state
        CHECK (stock_state IN ('GOOD', 'PENDING_INSPECTION', 'DEFECTIVE')),

    CONSTRAINT stock_movement_line_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(movement_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(location_code) <> ''
            AND btrim(goods_code) <> ''
            AND btrim(unit_name) <> ''
            AND btrim(inbound_batch_id) <> ''
        ),

    CONSTRAINT stock_movement_line_optional_text
        CHECK (
            (label_alias IS NULL OR btrim(label_alias) <> '')
            AND (batch_number IS NULL OR btrim(batch_number) <> '')
        ),

    -- 声明的入库时间只属于记入行。取走记成负的 quantity_delta，带了声明时间就不是一笔记入。
    CONSTRAINT stock_movement_line_declared_inbound
        CHECK (declared_inbound_at IS NULL OR quantity_delta > 0)
);

-- 算一格余额时按这些列收窄。
CREATE INDEX stock_movement_line_cell_idx
    ON warehouse_fulfillment.stock_movement_line (
        tenant_id, owner_account_id, warehouse_code, goods_code, location_code, inbound_batch_id
    );

-- 「这个商品有没有在库库存」按货主与商品收窄，不限仓。
CREATE INDEX stock_movement_line_goods_idx
    ON warehouse_fulfillment.stock_movement_line (tenant_id, owner_account_id, goods_code);
