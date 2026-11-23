-- 出库确认（UC-WF-015）。面单本体没有存放，在线的本产品网络因此答未配置。
-- 拣货落进的待出库库位记在任务上，确认时按包裹数量从那里取走。

ALTER TABLE warehouse_fulfillment.pick_task
    ADD COLUMN staging_location text;

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_status;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'SPLIT',
        'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED'
    ));

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN confirmation_state text,
    ADD CONSTRAINT outbound_parcel_confirmation_state CHECK (
        confirmation_state IS NULL OR confirmation_state IN ('CONFIRMED', 'RECORDED')
    );

CREATE TABLE warehouse_fulfillment.outbound_confirmation (
    tenant_id          text        NOT NULL,
    fact_id            text        NOT NULL,
    request_id         text        NOT NULL,
    parcel_sequence    integer     NOT NULL,
    kind               text        NOT NULL,
    offline            boolean     NOT NULL,
    missing            text        NOT NULL,
    external_number    text,
    pickup_party       text,
    pickup_credential  text,
    occurred_at        timestamptz NOT NULL,

    CONSTRAINT outbound_confirmation_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT outbound_confirmation_sequence CHECK (parcel_sequence >= 1),
    CONSTRAINT outbound_confirmation_kind CHECK (kind IN ('OUTBOUND_CONFIRMATION', 'PICKUP_HANDOVER')),
    CONSTRAINT outbound_confirmation_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(fact_id) <> '' AND btrim(request_id) <> ''
        AND (external_number IS NULL OR btrim(external_number) <> '')
        AND (pickup_party IS NULL OR btrim(pickup_party) <> '')
        AND (pickup_credential IS NULL OR btrim(pickup_credential) <> '')
    )
);

CREATE TABLE warehouse_fulfillment.outbound_owner_label (
    tenant_id        text NOT NULL,
    request_id       text NOT NULL,
    parcel_sequence  integer NOT NULL,
    external_number  text NOT NULL,

    CONSTRAINT outbound_owner_label_pkey PRIMARY KEY (tenant_id, request_id, parcel_sequence),
    CONSTRAINT outbound_owner_label_sequence CHECK (parcel_sequence >= 1),
    CONSTRAINT outbound_owner_label_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(request_id) <> '' AND btrim(external_number) <> ''
    )
);
