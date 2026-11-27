-- 出库序列号只挂在打出的包裹上，不进库存格。
-- 商品修订加一格是否跟踪序列号，缺省不跟踪；在库时也能改，因为它不参与库存身份。
-- 同一张出库单里号不重复。旧包裹没有号，不补行。

ALTER TABLE warehouse_fulfillment.goods_revision
    ADD COLUMN track_serial boolean NOT NULL DEFAULT false;

CREATE TABLE warehouse_fulfillment.outbound_parcel_serial (
    tenant_id       text    NOT NULL,
    request_id      text    NOT NULL,
    parcel_sequence integer NOT NULL,
    serial_index    integer NOT NULL,
    serial_number   text    NOT NULL,

    CONSTRAINT outbound_parcel_serial_pkey
        PRIMARY KEY (tenant_id, request_id, parcel_sequence, serial_index),
    CONSTRAINT outbound_parcel_serial_unique
        UNIQUE (tenant_id, request_id, serial_number),
    CONSTRAINT outbound_parcel_serial_index CHECK (serial_index >= 0),
    CONSTRAINT outbound_parcel_serial_not_blank CHECK (btrim(serial_number) <> ''),
    CONSTRAINT outbound_parcel_serial_parcel_fkey
        FOREIGN KEY (tenant_id, request_id, parcel_sequence)
        REFERENCES warehouse_fulfillment.outbound_parcel (tenant_id, request_id, parcel_sequence)
);
