-- 按（租户，声明的段）列出移动事实（票 demo-chain-gaps/53，ADR-0224 决定二）。
--
-- 事实可以先于它声明的段登记（0026、ADR-0205 决定四）；对象日后凭交接进段时，全程追踪要按段找回这些事实补派生。没有本索引，
-- 每一次交接都会让那一问扫全表。只覆盖声明了段的行：没声明段的事实永远不会被按段问到。
CREATE INDEX transport_movement_fact_by_segment
    ON transport_fulfillment.transport_movement_fact (tenant_id, segment_ref, fact_ref, recorded_at, version)
    WHERE segment_ref IS NOT NULL;
