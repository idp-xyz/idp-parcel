-- 收货确认的核心（商品入库）。一条收货事实对着一份入库预报：成立的行另以收货流水入账，差异记在本表。
-- 事实身份由设备签发，在租户内唯一。差异标识由本上下文签发。
-- 到达先后用 received_seq，不靠设备上的发生时刻：同一份预报上谁先入账，以本表插入的先后为准。
--
-- 本表不对预报、库位、商品的修订表建外键。行和差异只指向本迁移自己的事实头。
-- 库上守闭集、数量为正，以及可空文本是 NULL 或非空白。
--
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。

CREATE TABLE warehouse_fulfillment.receipt_confirmation (
    tenant_id      text        NOT NULL,
    fact_id        text        NOT NULL,
    device_id      text        NOT NULL,
    operator_id    text        NOT NULL,
    occurred_at    timestamptz NOT NULL,
    received_at    timestamptz NOT NULL,
    received_seq   bigint      GENERATED ALWAYS AS IDENTITY,
    warehouse_code text        NOT NULL,
    pre_advice_id  text        NOT NULL,
    inbound_mode   text        NOT NULL,
    outcome        text        NOT NULL,

    CONSTRAINT receipt_confirmation_pkey
        PRIMARY KEY (tenant_id, fact_id),

    CONSTRAINT receipt_confirmation_received_seq_key
        UNIQUE (received_seq),

    CONSTRAINT receipt_confirmation_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(device_id) <> ''
            AND btrim(operator_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(pre_advice_id) <> ''
        ),

    CONSTRAINT receipt_confirmation_inbound_mode
        CHECK (inbound_mode IN ('BY_GOODS', 'BY_CARTON')),

    CONSTRAINT receipt_confirmation_outcome
        CHECK (outcome IN ('CONFIRMED', 'NOT_RECEIVABLE'))
);

CREATE INDEX receipt_confirmation_pre_advice_idx
    ON warehouse_fulfillment.receipt_confirmation (tenant_id, pre_advice_id, received_seq);

CREATE TABLE warehouse_fulfillment.receipt_confirmation_line (
    tenant_id        text        NOT NULL,
    fact_id          text        NOT NULL,
    line_index       integer     NOT NULL,
    observed_text    text        NOT NULL,
    unit_name        text        NOT NULL,
    stock_state      text        NOT NULL,
    quantity         bigint      NOT NULL,
    batch_number     text,
    expiry_date      date,
    label_alias      text,
    location_code    text        NOT NULL,
    goods_code       text,
    result           text,
    unposted_reason  text,

    CONSTRAINT receipt_confirmation_line_pkey
        PRIMARY KEY (tenant_id, fact_id, line_index),

    CONSTRAINT receipt_confirmation_line_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.receipt_confirmation (tenant_id, fact_id),

    CONSTRAINT receipt_confirmation_line_not_blank
        CHECK (
            btrim(observed_text) <> ''
            AND btrim(unit_name) <> ''
            AND btrim(location_code) <> ''
            AND line_index >= 0
        ),

    CONSTRAINT receipt_confirmation_line_quantity
        CHECK (quantity > 0),

    CONSTRAINT receipt_confirmation_line_state
        CHECK (stock_state IN ('GOOD', 'PENDING_INSPECTION', 'DEFECTIVE')),

    CONSTRAINT receipt_confirmation_line_optional_text
        CHECK (
            (batch_number IS NULL OR btrim(batch_number) <> '')
            AND (label_alias IS NULL OR btrim(label_alias) <> '')
            AND (goods_code IS NULL OR btrim(goods_code) <> '')
        ),

    CONSTRAINT receipt_confirmation_line_result_shape
        CHECK (
            (
                result = 'POSTED'
                AND goods_code IS NOT NULL
                AND unposted_reason IS NULL
            )
            OR (
                result = 'UNKNOWN_GOODS'
                AND goods_code IS NULL
                AND unposted_reason = 'UNKNOWN_GOODS'
            )
            OR (
                result IS NULL
                AND goods_code IS NULL
                AND unposted_reason = 'NOT_RECEIVABLE'
            )
        )
);

CREATE TABLE warehouse_fulfillment.receipt_discrepancy (
    tenant_id       text    NOT NULL,
    discrepancy_id  text    NOT NULL,
    line_index      integer NOT NULL,
    fact_id         text    NOT NULL,
    kind            text    NOT NULL,
    quantity        bigint  NOT NULL,
    goods_code      text,
    observed_text   text,

    CONSTRAINT receipt_discrepancy_pkey
        PRIMARY KEY (tenant_id, discrepancy_id, line_index),

    CONSTRAINT receipt_discrepancy_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.receipt_confirmation (tenant_id, fact_id),

    CONSTRAINT receipt_discrepancy_fact_line_key
        UNIQUE (tenant_id, fact_id, line_index),

    CONSTRAINT receipt_discrepancy_not_blank
        CHECK (btrim(discrepancy_id) <> '' AND line_index >= 0),

    CONSTRAINT receipt_discrepancy_quantity
        CHECK (quantity > 0),

    CONSTRAINT receipt_discrepancy_kind
        CHECK (kind IN ('OVERAGE', 'WRONG_GOODS', 'DAMAGE')),

    CONSTRAINT receipt_discrepancy_subject
        CHECK (
            (
                kind IN ('OVERAGE', 'DAMAGE')
                AND goods_code IS NOT NULL
                AND btrim(goods_code) <> ''
                AND observed_text IS NULL
            )
            OR (
                kind = 'WRONG_GOODS'
                AND (
                    (
                        goods_code IS NOT NULL
                        AND btrim(goods_code) <> ''
                        AND observed_text IS NULL
                    )
                    OR (
                        goods_code IS NULL
                        AND observed_text IS NOT NULL
                        AND btrim(observed_text) <> ''
                    )
                )
            )
        )
);
