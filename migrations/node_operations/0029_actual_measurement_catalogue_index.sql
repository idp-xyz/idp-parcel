-- 实际测量册按登记时刻倒序翻页。索引形状与读查询的排序一致：租户、登记时刻、来源标识。

CREATE INDEX actual_measurement_catalogue_by_recorded
    ON node_operations.actual_measurement (tenant_id, recorded_at DESC, source_id DESC);
