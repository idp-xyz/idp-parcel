-- 放出后撤单：把一张出库请求从波次里撤出，它在这个波次里待执行的拣货任务作废，请求回到已预占。
-- 成员行留着，只记下谁在何时撤的：波次曾经放过哪些单仍然查得到，同一个波次号重放放波次仍答已有结果。

ALTER TABLE warehouse_fulfillment.wave_request
    ADD COLUMN withdrawn_at timestamptz,
    ADD COLUMN withdrawn_by text,
    ADD CONSTRAINT wave_request_withdrawal CHECK (
        (withdrawn_at IS NULL AND withdrawn_by IS NULL)
        OR (withdrawn_at IS NOT NULL AND withdrawn_by IS NOT NULL AND btrim(withdrawn_by) <> '')
    );
