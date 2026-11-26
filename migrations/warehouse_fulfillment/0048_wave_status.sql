-- 波次的状态落成一列。列表要按状态筛、要数各状态的张数；读的时候现算，就得给本租户每一个波次数一遍拣货任务，
-- 波次越积越慢。规则只有一条，与适配器里重算用的是同一条：只看待执行、拣满、少拣三种任务，作废与迟到的不算——
-- 有任务待执行、没有一条拣满或少拣是已放出；有任务待执行、已有拣满或少拣是拣货中；
-- 没有任务待执行、至少一条拣满或少拣是已完成；其余（任务全作废，或没有任务）是已取消。
-- 之后任务状态每变一次，写任务的那一笔事务里重算所属波次的这一列。

ALTER TABLE warehouse_fulfillment.wave
    ADD COLUMN status text;

UPDATE warehouse_fulfillment.wave AS target
   SET status = CASE WHEN tally.open_tasks > 0 AND tally.settled_tasks = 0 THEN 'RELEASED'
                     WHEN tally.open_tasks > 0 THEN 'PICKING'
                     WHEN tally.settled_tasks > 0 THEN 'COMPLETED'
                     ELSE 'CANCELLED' END
  FROM (
        SELECT released.tenant_id, released.wave_id,
               count(*) FILTER (WHERE task.status = 'OPEN') AS open_tasks,
               count(*) FILTER (WHERE task.status IN ('DONE', 'SHORT')) AS settled_tasks
          FROM warehouse_fulfillment.wave released
          LEFT JOIN warehouse_fulfillment.pick_task task
            ON task.tenant_id = released.tenant_id AND task.wave_id = released.wave_id
         GROUP BY released.tenant_id, released.wave_id
       ) tally
 WHERE tally.tenant_id = target.tenant_id AND tally.wave_id = target.wave_id;

ALTER TABLE warehouse_fulfillment.wave
    ALTER COLUMN status SET NOT NULL,
    ADD CONSTRAINT wave_status CHECK (status IN ('RELEASED', 'PICKING', 'COMPLETED', 'CANCELLED'));

-- 按状态筛的列表照放出时刻从新到旧翻页。
CREATE INDEX wave_status_idx
    ON warehouse_fulfillment.wave (tenant_id, status, released_at DESC, wave_id DESC);
