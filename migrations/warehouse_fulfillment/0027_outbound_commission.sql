-- 装箱记录记下以货主名义提交成的委托。空着就是还没提交：代办授予没登记时停在这一格。

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD COLUMN shipment_request_id text;

ALTER TABLE warehouse_fulfillment.outbound_parcel
    ADD CONSTRAINT outbound_parcel_shipment_request
        CHECK (shipment_request_id IS NULL OR btrim(shipment_request_id) <> '');
