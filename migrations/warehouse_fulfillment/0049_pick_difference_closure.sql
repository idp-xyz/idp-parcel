-- 少拣留下的拣货差异有了去处。差异待处理的那段时间里，它所在库位上的那种货不再分给拣货任务：
-- 账上说还有，拣货的人刚在那里确认过没有，再派人去只会再少拣一次。一份盘点把那个库位盘完，差异就了结——
-- 盘出来少了由盘点差异去调账，盘出来货在，那个库位照常再分配。
-- 记下的时刻只有新的差异有：已有的差异落册时没有记时刻，不替它补一个。

ALTER TABLE warehouse_fulfillment.pick_difference
    DROP CONSTRAINT pick_difference_status;

ALTER TABLE warehouse_fulfillment.pick_difference
    ADD COLUMN recorded_at timestamptz,
    ADD COLUMN closed_at timestamptz,
    ADD COLUMN closed_by_count text,
    ADD CONSTRAINT pick_difference_status CHECK (status IN ('PENDING', 'CLOSED')),
    ADD CONSTRAINT pick_difference_closure CHECK (
        (status = 'PENDING' AND closed_at IS NULL AND closed_by_count IS NULL)
        OR (status = 'CLOSED' AND closed_at IS NOT NULL AND closed_by_count IS NOT NULL AND btrim(closed_by_count) <> '')
    );

-- 分配时按任务找待处理的差异；了结的越积越多，不进这条索引。
CREATE INDEX pick_difference_pending_idx
    ON warehouse_fulfillment.pick_difference (tenant_id, task_id)
 WHERE status = 'PENDING';
