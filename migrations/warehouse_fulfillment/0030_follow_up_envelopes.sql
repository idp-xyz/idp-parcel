-- 装箱记录记下提交时现签的声明包裹，供补充与取消对上。
-- 资料版本的采用判断与包裹取消各记一笔。更正未采用、取消成立，都不作出库确认。

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN declared_parcel_id text;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN correction_state text;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    DROP CONSTRAINT outbound_parcel_commission_state;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_declared_parcel CHECK (
        declared_parcel_id IS NULL OR btrim(declared_parcel_id) <> ''
    );

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_correction_state CHECK (
        correction_state IS NULL OR correction_state IN ('ADOPTED', 'AWAITING_REVIEW')
    );

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_commission_state CHECK (
        commission_state IS NULL
        OR (commission_state IN ('ACCEPTED', 'REJECTED', 'AWAITING_SUPPLEMENT', 'CANCELLED') AND shipment_request_id IS NOT NULL)
    );

CREATE TABLE warehouse_fulfillment.source_data_posting (
    tenant_id          text        NOT NULL,
    version_id         text        NOT NULL,
    source             text        NOT NULL,
    source_request_key text        NOT NULL,
    adoption           text        NOT NULL,
    posted_at          timestamptz NOT NULL,

    CONSTRAINT source_data_posting_pkey PRIMARY KEY (tenant_id, version_id),
    CONSTRAINT source_data_posting_adoption CHECK (adoption IN ('ADOPTED', 'AWAITING_REVIEW')),
    CONSTRAINT source_data_posting_not_blank CHECK (
        btrim(version_id) <> '' AND btrim(source) <> '' AND btrim(source_request_key) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.cancellation_posting (
    tenant_id   text        NOT NULL,
    request_key text        NOT NULL,
    parcel_id   text        NOT NULL,
    matched     boolean     NOT NULL,
    posted_at   timestamptz NOT NULL,

    CONSTRAINT cancellation_posting_pkey PRIMARY KEY (tenant_id, request_key, parcel_id),
    CONSTRAINT cancellation_posting_not_blank CHECK (
        btrim(request_key) <> '' AND btrim(parcel_id) <> ''
    )
);
