-- 「委托待受控补充」的入账。来源不是仓配出库的只记一行。
-- 仓配出库的把对应包裹标成待货主补正。装箱记录的状态检查放开这一格。

ALTER TABLE warehouse_fulfillment.outbound_parcel
    DROP CONSTRAINT outbound_parcel_commission_state;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_commission_state CHECK (
        commission_state IS NULL
        OR (commission_state IN ('ACCEPTED', 'REJECTED', 'AWAITING_SUPPLEMENT') AND shipment_request_id IS NOT NULL)
    );

CREATE TABLE warehouse_fulfillment.supplement_posting (
    tenant_id              text        NOT NULL,
    submission_version_id  text        NOT NULL,
    source                 text        NOT NULL,
    source_request_key     text        NOT NULL,
    posted_at              timestamptz NOT NULL,

    CONSTRAINT supplement_posting_pkey PRIMARY KEY (tenant_id, submission_version_id),
    CONSTRAINT supplement_posting_not_blank CHECK (
        btrim(submission_version_id) <> '' AND btrim(source) <> '' AND btrim(source_request_key) <> ''
    )
);
