-- 库存冻结（UC-WF-009）。冻结不记流水，只占可用量，整份解冻。
-- 本迁移只建结构、不种任何行。

CREATE TABLE warehouse_fulfillment.stock_freeze (
    tenant_id           text        NOT NULL,
    freeze_id           text        NOT NULL,
    decision_id         text        NOT NULL,
    owner_account_id    text        NOT NULL,
    warehouse_code      text        NOT NULL,
    target              text        NOT NULL,
    location_code       text,
    goods_code          text,
    unit_name           text,
    stock_state         text,
    label_alias         text,
    inbound_batch_id    text,
    batch_number        text,
    expiry_date         date,
    carton_number       text,
    quantity            bigint      NOT NULL,
    reason              text        NOT NULL,
    operator_id         text        NOT NULL,
    frozen_at           timestamptz NOT NULL,
    status              text        NOT NULL,
    unfreeze_operator_id text,
    unfrozen_at         timestamptz,
    unfreeze_note       text,

    CONSTRAINT stock_freeze_pkey PRIMARY KEY (tenant_id, freeze_id),
    CONSTRAINT stock_freeze_decision UNIQUE (tenant_id, decision_id),
    CONSTRAINT stock_freeze_target CHECK (target IN ('GOODS', 'CARTON')),
    CONSTRAINT stock_freeze_status CHECK (status IN ('FROZEN', 'UNFROZEN')),
    CONSTRAINT stock_freeze_quantity CHECK (quantity > 0),
    CONSTRAINT stock_freeze_goods_shape CHECK (
        target <> 'GOODS' OR (
            location_code IS NOT NULL AND goods_code IS NOT NULL AND unit_name IS NOT NULL
            AND stock_state IS NOT NULL AND inbound_batch_id IS NOT NULL AND carton_number IS NULL
        )
    ),
    CONSTRAINT stock_freeze_carton_shape CHECK (
        target <> 'CARTON' OR (
            carton_number IS NOT NULL AND quantity = 1
            AND location_code IS NULL AND goods_code IS NULL AND unit_name IS NULL
            AND stock_state IS NULL AND inbound_batch_id IS NULL
        )
    ),
    CONSTRAINT stock_freeze_open CHECK (
        (status = 'FROZEN' AND unfrozen_at IS NULL AND unfreeze_operator_id IS NULL)
        OR (status = 'UNFROZEN' AND unfrozen_at IS NOT NULL AND unfreeze_operator_id IS NOT NULL)
    ),
    CONSTRAINT stock_freeze_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(freeze_id) <> '' AND btrim(decision_id) <> ''
        AND btrim(owner_account_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(reason) <> '' AND btrim(operator_id) <> ''
        AND (location_code IS NULL OR btrim(location_code) <> '')
        AND (goods_code IS NULL OR btrim(goods_code) <> '')
        AND (carton_number IS NULL OR btrim(carton_number) <> '')
        AND (unfreeze_note IS NULL OR btrim(unfreeze_note) <> '')
    )
);
