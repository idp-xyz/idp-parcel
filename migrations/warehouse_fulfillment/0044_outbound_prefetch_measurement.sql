-- 放波次预取记下估算时的重量与外廓。打包称重与它相同时直接沿用，不同时按资料修订更正。
ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN prefetch_weight text,
    ADD COLUMN prefetch_weight_unit text,
    ADD COLUMN prefetch_length text,
    ADD COLUMN prefetch_width text,
    ADD COLUMN prefetch_height text,
    ADD COLUMN prefetch_dimension_unit text;
