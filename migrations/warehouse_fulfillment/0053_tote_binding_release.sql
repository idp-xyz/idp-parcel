-- 周转箱的绑定多数是系统放开的：出库请求打完包，或截单的货都回了库。箱被拿错、被清空而系统不知道时，
-- 由人放开；那一次是谁放的记在 released_by 上。系统放开的这一格留空。
--
-- 主键换成一个只增的序号：一个箱放开之后可以在同一刻再绑上（放开与下一次拣货落在同一个时刻），
-- 拿绑上的时刻当主键的一段会把这第二次绑定挡在门外。「同一时刻一个箱只属于一张请求」仍由
-- tote_binding_live_idx 守着，与主键无关。

ALTER TABLE warehouse_fulfillment.tote_binding
    DROP CONSTRAINT tote_binding_pkey,
    ADD COLUMN binding_seq bigint GENERATED ALWAYS AS IDENTITY,
    ADD CONSTRAINT tote_binding_pkey PRIMARY KEY (tenant_id, binding_seq),
    ADD COLUMN released_by text,
    ADD CONSTRAINT tote_binding_released_by CHECK (released_by IS NULL OR released_at IS NOT NULL);
