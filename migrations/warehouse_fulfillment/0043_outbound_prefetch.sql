-- 放波次预取记下已经交出去的那份委托。打包后沿用它，不再按同一包裹序号另交一份。
ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN prefetch_shipment_id text,
    ADD COLUMN prefetch_declared_parcel_id text;
