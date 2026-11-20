-- 出库请求与预占（UC-WF-012）。受理在同一事务取单据号。预占不落库位，不写库存流水。
-- 缺货处置按货主登记，本迁移不种任何行。

CREATE TABLE warehouse_fulfillment.outbound_request (
    tenant_id          text        NOT NULL,
    request_id         text        NOT NULL,
    owner_account_id   text        NOT NULL,
    warehouse_code     text        NOT NULL,
    origin             text        NOT NULL,
    request_key        text        NOT NULL,
    dispatch           text        NOT NULL,
    service_product    text,
    recipient          text        NOT NULL,
    address            text        NOT NULL,
    basis              text,
    operator_id        text,
    status             text        NOT NULL,
    document_number    text,
    numbering_absence  text,
    received_at        timestamptz NOT NULL,

    CONSTRAINT outbound_request_pkey PRIMARY KEY (tenant_id, request_id),
    CONSTRAINT outbound_request_key UNIQUE (tenant_id, origin, request_key),
    CONSTRAINT outbound_request_origin CHECK (origin IN ('CUSTOMER', 'OPERATOR')),
    CONSTRAINT outbound_request_dispatch CHECK (dispatch IN ('PRODUCT_NETWORK', 'OWNER_LABEL', 'PICKUP')),
    CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF'
    )),
    CONSTRAINT outbound_request_number_shape CHECK (
        (document_number IS NOT NULL AND numbering_absence IS NULL)
        OR (document_number IS NULL AND numbering_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED')
    ),
    CONSTRAINT outbound_request_operator_shape CHECK (
        (origin = 'CUSTOMER' AND basis IS NULL AND operator_id IS NULL)
        OR (origin = 'OPERATOR' AND basis IS NOT NULL AND operator_id IS NOT NULL)
    ),
    CONSTRAINT outbound_request_network_service CHECK (
        dispatch <> 'PRODUCT_NETWORK' OR service_product IS NOT NULL
    ),
    CONSTRAINT outbound_request_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(request_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(request_key) <> ''
        AND btrim(recipient) <> '' AND btrim(address) <> ''
        AND (service_product IS NULL OR btrim(service_product) <> '')
        AND (basis IS NULL OR btrim(basis) <> '')
        AND (operator_id IS NULL OR btrim(operator_id) <> '')
        AND (document_number IS NULL OR btrim(document_number) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.outbound_request_line (
    tenant_id    text    NOT NULL,
    request_id   text    NOT NULL,
    line_index   integer NOT NULL,
    goods_code   text    NOT NULL,
    unit_name    text    NOT NULL,
    stock_state  text    NOT NULL,
    label_alias  text,
    batch_number text,
    expiry_date  date,
    quantity     bigint  NOT NULL,

    CONSTRAINT outbound_request_line_pkey PRIMARY KEY (tenant_id, request_id, line_index),
    CONSTRAINT outbound_request_line_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_request_line_quantity CHECK (quantity > 0 AND line_index >= 0),
    CONSTRAINT outbound_request_line_not_blank CHECK (
        btrim(goods_code) <> '' AND btrim(unit_name) <> '' AND btrim(stock_state) <> ''
        AND (label_alias IS NULL OR btrim(label_alias) <> '')
        AND (batch_number IS NULL OR btrim(batch_number) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.outbound_reservation (
    tenant_id       text    NOT NULL,
    request_id      text    NOT NULL,
    line_index      integer NOT NULL,
    goods_code      text    NOT NULL,
    stock_state     text    NOT NULL,
    label_alias     text,
    batch_number    text,
    expiry_date     date,
    quantity        bigint  NOT NULL,
    from_unlabeled  bigint  NOT NULL,
    wanted          bigint  NOT NULL,

    CONSTRAINT outbound_reservation_pkey PRIMARY KEY (tenant_id, request_id, line_index),
    CONSTRAINT outbound_reservation_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_reservation_quantity CHECK (
        quantity >= 0 AND from_unlabeled >= 0 AND from_unlabeled <= quantity AND wanted > 0 AND line_index >= 0
    )
);

CREATE TABLE warehouse_fulfillment.shortage_policy (
    tenant_id        text        NOT NULL,
    owner_account_id text        NOT NULL,
    revision         integer     NOT NULL,
    kind             text        NOT NULL,
    operator_id      text        NOT NULL,
    registered_at    timestamptz NOT NULL,

    CONSTRAINT shortage_policy_pkey PRIMARY KEY (tenant_id, owner_account_id, revision),
    CONSTRAINT shortage_policy_kind CHECK (kind IN ('PARTIAL', 'REJECT')),
    CONSTRAINT shortage_policy_revision CHECK (revision >= 1),
    CONSTRAINT shortage_policy_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(owner_account_id) <> '' AND btrim(operator_id) <> ''
    )
);
