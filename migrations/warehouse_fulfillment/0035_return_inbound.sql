-- 退货预报、退货入库与回库处置（UC-WF-017）。预报不是库存。入库进入退货暂存并形成新批次。
-- 处置只记下去向，不代替上架、转次品、销毁或退还货主的流水。

CREATE TABLE warehouse_fulfillment.return_pre_advice (
    tenant_id         text        NOT NULL,
    advice_id         text        NOT NULL,
    owner_account_id  text        NOT NULL,
    warehouse_code    text        NOT NULL,
    origin            text        NOT NULL,
    request_key       text        NOT NULL,
    tracking_number   text        NOT NULL,
    basis             text,
    operator_id       text,
    document_number   text,
    numbering_absence text,
    received_at       timestamptz NOT NULL,

    CONSTRAINT return_pre_advice_pkey PRIMARY KEY (tenant_id, advice_id),
    CONSTRAINT return_pre_advice_key UNIQUE (tenant_id, origin, request_key),
    CONSTRAINT return_pre_advice_origin CHECK (origin IN ('CUSTOMER', 'OPERATOR')),
    CONSTRAINT return_pre_advice_number_shape CHECK (
        (document_number IS NOT NULL AND numbering_absence IS NULL)
        OR (document_number IS NULL AND numbering_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED')
    ),
    CONSTRAINT return_pre_advice_operator_shape CHECK (
        (origin = 'CUSTOMER' AND basis IS NULL AND operator_id IS NULL)
        OR (origin = 'OPERATOR' AND basis IS NOT NULL AND operator_id IS NOT NULL)
    ),
    CONSTRAINT return_pre_advice_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(advice_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(request_key) <> '' AND btrim(tracking_number) <> ''
        AND (basis IS NULL OR btrim(basis) <> '')
        AND (operator_id IS NULL OR btrim(operator_id) <> '')
        AND (document_number IS NULL OR btrim(document_number) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.return_pre_advice_line (
    tenant_id  text    NOT NULL,
    advice_id  text    NOT NULL,
    line_index integer NOT NULL,
    goods_code text    NOT NULL,
    unit_name  text    NOT NULL,
    quantity   bigint  NOT NULL,

    CONSTRAINT return_pre_advice_line_pkey PRIMARY KEY (tenant_id, advice_id, line_index),
    CONSTRAINT return_pre_advice_line_advice_fkey
        FOREIGN KEY (tenant_id, advice_id)
        REFERENCES warehouse_fulfillment.return_pre_advice (tenant_id, advice_id),
    CONSTRAINT return_pre_advice_line_quantity CHECK (quantity > 0 AND line_index >= 0),
    CONSTRAINT return_pre_advice_line_not_blank CHECK (btrim(goods_code) <> '' AND btrim(unit_name) <> '')
);

CREATE TABLE warehouse_fulfillment.return_receipt (
    tenant_id        text        NOT NULL,
    fact_id          text        NOT NULL,
    warehouse_code   text        NOT NULL,
    location_code    text        NOT NULL,
    tracking_number  text        NOT NULL,
    owner_account_id text,
    advice_id        text,
    arrival_id       text,
    goods_code       text        NOT NULL,
    unit_name        text        NOT NULL,
    stock_state      text        NOT NULL,
    quantity         bigint      NOT NULL,
    outcome          text        NOT NULL,
    occurred_at      timestamptz NOT NULL,

    CONSTRAINT return_receipt_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT return_receipt_quantity CHECK (quantity > 0),
    CONSTRAINT return_receipt_outcome CHECK (outcome IN ('COUNTED', 'UNCLAIMED')),
    CONSTRAINT return_receipt_shape CHECK (
        (outcome = 'COUNTED' AND owner_account_id IS NOT NULL AND btrim(owner_account_id) <> '')
        OR (outcome = 'UNCLAIMED' AND owner_account_id IS NULL AND arrival_id IS NOT NULL)
    ),
    CONSTRAINT return_receipt_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(location_code) <> '' AND btrim(tracking_number) <> ''
        AND btrim(goods_code) <> '' AND btrim(unit_name) <> '' AND btrim(stock_state) <> ''
        AND (advice_id IS NULL OR btrim(advice_id) <> '')
        AND (arrival_id IS NULL OR btrim(arrival_id) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.return_disposition (
    tenant_id    text        NOT NULL,
    decision_id  text        NOT NULL,
    fact_id      text        NOT NULL,
    destination  text        NOT NULL,
    basis        text,
    operator_id  text,
    origin       text        NOT NULL,
    decided_at   timestamptz NOT NULL,

    CONSTRAINT return_disposition_pkey PRIMARY KEY (tenant_id, decision_id),
    CONSTRAINT return_disposition_destination CHECK (destination IN ('GOOD_PUTAWAY', 'DEFECTIVE', 'DESTROY', 'RETURN_TO_OWNER')),
    CONSTRAINT return_disposition_origin CHECK (origin IN ('CUSTOMER', 'OPERATOR')),
    CONSTRAINT return_disposition_operator_shape CHECK (
        (origin = 'CUSTOMER' AND basis IS NULL AND operator_id IS NULL)
        OR (origin = 'OPERATOR' AND basis IS NOT NULL AND operator_id IS NOT NULL)
    ),
    CONSTRAINT return_disposition_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(decision_id) <> '' AND btrim(fact_id) <> ''
    )
);
