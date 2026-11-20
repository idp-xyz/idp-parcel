-- 建账导入（UC-WF-006）。提交落成待批准，不改账。批准成立后各行同笔入账；驳回不改账。
-- 本迁移不种任何行。谁能提交、谁能批准是租户取值。
--
-- 整箱流水补上建账导入这一种原因。收货、库内移动与开箱的原因留着，不把建账导入写成其中一种。

ALTER TABLE warehouse_fulfillment.carton_movement
    DROP CONSTRAINT carton_movement_reason;

ALTER TABLE warehouse_fulfillment.carton_movement
    ADD CONSTRAINT carton_movement_reason
        CHECK (reason IN ('RECEIPT', 'INTERNAL_MOVE', 'UNBOXING', 'TAKE_ON_IMPORT'));

CREATE TABLE warehouse_fulfillment.stock_import (
    tenant_id          text        NOT NULL,
    import_id          text        NOT NULL,
    owner_account_id   text        NOT NULL,
    warehouse_code     text        NOT NULL,
    reason             text        NOT NULL,
    status             text        NOT NULL,
    submitter          text        NOT NULL,
    submitted_at       timestamptz NOT NULL,
    poster             text,
    posted_at          timestamptz,
    goods_movement_id  text,
    carton_movement_id text,
    rejector           text,
    rejected_at        timestamptz,
    rejection_note     text,

    CONSTRAINT stock_import_pkey
        PRIMARY KEY (tenant_id, import_id),

    CONSTRAINT stock_import_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(import_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(reason) <> ''
            AND btrim(submitter) <> ''
            AND (poster IS NULL OR btrim(poster) <> '')
            AND (goods_movement_id IS NULL OR btrim(goods_movement_id) <> '')
            AND (carton_movement_id IS NULL OR btrim(carton_movement_id) <> '')
            AND (rejector IS NULL OR btrim(rejector) <> '')
            AND (rejection_note IS NULL OR btrim(rejection_note) <> '')
        ),

    -- 可空列与状态必须成组出现。NULL 比较是 UNKNOWN，CHECK 会放行，所以每一支都写 IS NULL / IS NOT NULL。
    CONSTRAINT stock_import_status_shape
        CHECK (
            (
                status = 'PENDING'
                AND poster IS NULL
                AND posted_at IS NULL
                AND goods_movement_id IS NULL
                AND carton_movement_id IS NULL
                AND rejector IS NULL
                AND rejected_at IS NULL
                AND rejection_note IS NULL
            )
            OR (
                status = 'POSTED'
                AND poster IS NOT NULL
                AND posted_at IS NOT NULL
                AND rejector IS NULL
                AND rejected_at IS NULL
                AND rejection_note IS NULL
                AND (goods_movement_id IS NOT NULL OR carton_movement_id IS NOT NULL)
            )
            OR (
                status = 'REJECTED'
                AND rejector IS NOT NULL
                AND rejected_at IS NOT NULL
                AND poster IS NULL
                AND posted_at IS NULL
                AND goods_movement_id IS NULL
                AND carton_movement_id IS NULL
            )
        )
);

CREATE INDEX stock_import_list_idx
    ON warehouse_fulfillment.stock_import (tenant_id, submitted_at DESC, import_id DESC);

CREATE TABLE warehouse_fulfillment.stock_import_goods_line (
    tenant_id            text        NOT NULL,
    import_id            text        NOT NULL,
    position             integer     NOT NULL,
    location_code        text        NOT NULL,
    goods_code           text        NOT NULL,
    unit_name            text        NOT NULL,
    stock_state          text        NOT NULL,
    quantity             bigint      NOT NULL,
    batch_number         text,
    expiry_date          date,
    label_alias          text,
    declared_inbound_at  timestamptz,

    CONSTRAINT stock_import_goods_line_pkey
        PRIMARY KEY (tenant_id, import_id, position),

    CONSTRAINT stock_import_goods_line_fkey
        FOREIGN KEY (tenant_id, import_id)
        REFERENCES warehouse_fulfillment.stock_import (tenant_id, import_id),

    CONSTRAINT stock_import_goods_line_shape
        CHECK (
            position >= 0
            AND quantity > 0
            AND btrim(location_code) <> ''
            AND btrim(goods_code) <> ''
            AND btrim(unit_name) <> ''
            AND stock_state IN ('GOOD', 'PENDING_INSPECTION', 'DEFECTIVE')
            AND (batch_number IS NULL OR btrim(batch_number) <> '')
            AND (label_alias IS NULL OR btrim(label_alias) <> '')
        )
);

CREATE TABLE warehouse_fulfillment.stock_import_carton_line (
    tenant_id           text        NOT NULL,
    import_id           text        NOT NULL,
    position            integer     NOT NULL,
    location_code       text        NOT NULL,
    carton_number       text        NOT NULL,
    declared_inbound_at timestamptz,

    CONSTRAINT stock_import_carton_line_pkey
        PRIMARY KEY (tenant_id, import_id, position),

    CONSTRAINT stock_import_carton_line_number
        UNIQUE (tenant_id, import_id, carton_number),

    CONSTRAINT stock_import_carton_line_fkey
        FOREIGN KEY (tenant_id, import_id)
        REFERENCES warehouse_fulfillment.stock_import (tenant_id, import_id),

    CONSTRAINT stock_import_carton_line_shape
        CHECK (
            position >= 0
            AND btrim(location_code) <> ''
            AND btrim(carton_number) <> ''
        )
);

CREATE TABLE warehouse_fulfillment.stock_import_carton_content (
    tenant_id        text    NOT NULL,
    import_id        text    NOT NULL,
    line_position    integer NOT NULL,
    content_position integer NOT NULL,
    goods_code       text    NOT NULL,
    quantity         bigint  NOT NULL,

    CONSTRAINT stock_import_carton_content_pkey
        PRIMARY KEY (tenant_id, import_id, line_position, content_position),

    CONSTRAINT stock_import_carton_content_fkey
        FOREIGN KEY (tenant_id, import_id, line_position)
        REFERENCES warehouse_fulfillment.stock_import_carton_line (tenant_id, import_id, position),

    CONSTRAINT stock_import_carton_content_shape
        CHECK (
            line_position >= 0
            AND content_position >= 0
            AND quantity > 0
            AND btrim(goods_code) <> ''
        )
);
