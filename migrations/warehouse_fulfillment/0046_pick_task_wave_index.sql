-- 波次的状态读的时候从它的拣货任务数出来，波次列表与按号读一张都按（租户，波次）取任务。
-- 拣货任务的主键只有（租户，任务），没有这一条时每数一个波次都要扫过本租户的全部任务。

CREATE INDEX pick_task_wave_idx
    ON warehouse_fulfillment.pick_task (tenant_id, wave_id);
