-- 派工：一个波次派给一个人拣。assignee 是拣货人的操作者标识（发行方#sub，与各册上记操作者的是同一个串）；
-- 空着是没派，谁都可以拣。由管波次的人指派，或拣货的人在工位上自己领；assigned_by 记是谁派的（自己领的就是他自己）。
-- 派给了一个人之后，别人拣这个波次里待执行的任务不成立。派工到波次为止，不到单条任务，也不按库区拆。

ALTER TABLE warehouse_fulfillment.wave
    ADD COLUMN assignee    text,
    ADD COLUMN assigned_by text,
    ADD COLUMN assigned_at timestamptz,
    ADD CONSTRAINT wave_assignment_whole CHECK (
        (assignee IS NULL AND assigned_by IS NULL AND assigned_at IS NULL)
        OR (assignee IS NOT NULL AND btrim(assignee) <> '' AND assigned_by IS NOT NULL AND assigned_at IS NOT NULL)
    );
