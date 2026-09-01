-- 接受判断任务等待态的查询投影：给「等待人工复核的都有谁」队列读面用（票
-- admin-skeleton-closure-batch/09，形状沿 ADR-0060 的同行投影先例）。
--
-- 不另建投影表，也不对 snapshot jsonb 建表达式索引：本列是当前任务 waitingOn 的
-- 查询投影，由 Insert/Save 与 snapshot 同一条 SQL 写下；分两次写会在两次之间把一份
-- 已完成复核的委托继续列在队列里。
--
-- 回填走 snapshot.acceptanceTask.waitingOn（适配器写下的形状，uint8，omitempty——
-- 不在等待即字段缺席，回填成 0）。取值域镜像领域 ResumePath 的封闭集合：
-- 0 = 无等待，1 = CUSTOMER_SUPPLEMENT，2 = INTERNAL_RETRY，3 = MANUAL_REVIEW。

ALTER TABLE parcel_shipment.shipment_request
    ADD COLUMN task_waiting_on smallint;

UPDATE parcel_shipment.shipment_request
   SET task_waiting_on = COALESCE((snapshot #>> '{acceptanceTask,waitingOn}')::smallint, 0);

ALTER TABLE parcel_shipment.shipment_request
    ALTER COLUMN task_waiting_on SET NOT NULL,
    ADD CONSTRAINT shipment_request_task_waiting_on_known
        CHECK (task_waiting_on BETWEEN 0 AND 3);

-- 队列只认「已提交且等人工复核」（state=1 即 SUBMITTED，waiting=3 即 MANUAL_REVIEW），
-- 部分索引让其余行不进索引；按提交时间序出队。
CREATE INDEX shipment_request_manual_review_queue
    ON parcel_shipment.shipment_request (tenant_id, submitted_at)
    WHERE task_waiting_on = 3 AND state = 1;
