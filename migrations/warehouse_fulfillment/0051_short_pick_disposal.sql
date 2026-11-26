-- 拣不齐的处置：一张出库请求少拣之后换不出别的库位，照货主的部分发货规则办的那一次。一张请求只办一次。
-- 少发（SHORT_SHIP）是拣到的发走、没拣到的不再发；拆单发（SPLIT_SHIP）是拣到的发走、没拣到的拆成另一张请求，
-- 那张请求记在 remainder_request_id 上。整单发与未配置都不办，所以这里只有两个词。
-- 记下当时照的是哪一层的规则、要发与实发各多少、谁在何时办的：规则之后可以改，这一次照的是什么得留着。

CREATE TABLE warehouse_fulfillment.outbound_short_pick_disposal (
    tenant_id               text        NOT NULL,
    request_id              text        NOT NULL,
    partial_shipment        text        NOT NULL,
    partial_shipment_source text        NOT NULL,
    wanted                  bigint      NOT NULL,
    shipped                 bigint      NOT NULL,
    remainder_request_id    text,
    operator_id             text        NOT NULL,
    recorded_at             timestamptz NOT NULL,

    CONSTRAINT outbound_short_pick_disposal_pkey PRIMARY KEY (tenant_id, request_id),
    CONSTRAINT outbound_short_pick_disposal_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_short_pick_disposal_remainder_fkey
        FOREIGN KEY (tenant_id, remainder_request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_short_pick_disposal_rule CHECK (partial_shipment IN ('SHORT_SHIP', 'SPLIT_SHIP')),
    CONSTRAINT outbound_short_pick_disposal_source CHECK (partial_shipment_source IN ('ORDER_CONSENT', 'ORDER', 'OWNER')),
    CONSTRAINT outbound_short_pick_disposal_quantities CHECK (shipped > 0 AND shipped < wanted),
    CONSTRAINT outbound_short_pick_disposal_remainder CHECK (
        (partial_shipment = 'SPLIT_SHIP') = (remainder_request_id IS NOT NULL)
    ),
    CONSTRAINT outbound_short_pick_disposal_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(request_id) <> '' AND btrim(operator_id) <> ''
    )
);
