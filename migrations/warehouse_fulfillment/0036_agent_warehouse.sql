-- 代理仓库存声明与转出（UC-WF-018）。声明不进库存账。连接器只记「这个仓用模拟源」，不存凭据。

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_status;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'FORWARDED', 'SPLIT',
        'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED',
        'CANCELLED', 'AWAITING_RETURN', 'PARTIALLY_CANCELLED'
    ));

CREATE TABLE warehouse_fulfillment.agent_connector (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT agent_connector_pkey PRIMARY KEY (tenant_id, warehouse_code),
    CONSTRAINT agent_connector_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '')
);

CREATE TABLE warehouse_fulfillment.agent_statement (
    tenant_id        text        NOT NULL,
    source_id        text        NOT NULL,
    stated_at        timestamptz NOT NULL,
    valid_until      timestamptz NOT NULL,
    warehouse_code   text        NOT NULL,
    owner_account_id text        NOT NULL,
    goods_code       text        NOT NULL,
    unit_name        text        NOT NULL,
    quantity         bigint      NOT NULL,

    CONSTRAINT agent_statement_pkey PRIMARY KEY (tenant_id, source_id, stated_at),
    CONSTRAINT agent_statement_quantity CHECK (quantity >= 0),
    CONSTRAINT agent_statement_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(source_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(owner_account_id) <> '' AND btrim(goods_code) <> '' AND btrim(unit_name) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.agent_result (
    tenant_id       text        NOT NULL,
    result_id       text        NOT NULL,
    request_id      text        NOT NULL,
    warehouse_code  text        NOT NULL,
    kind            text        NOT NULL,
    weight_value    text,
    weight_unit     text,
    length_value    text,
    width_value     text,
    height_value    text,
    dimension_unit  text,
    reported_at     timestamptz NOT NULL,

    CONSTRAINT agent_result_pkey PRIMARY KEY (tenant_id, result_id),
    CONSTRAINT agent_result_kind CHECK (kind IN ('FORWARDED', 'REJECTED', 'SHORT', 'PACKED', 'HANDED_OVER')),
    CONSTRAINT agent_result_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(result_id) <> '' AND btrim(request_id) <> '' AND btrim(warehouse_code) <> ''
    )
);
