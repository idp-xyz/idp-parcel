-- 回库任务可以做完：货从待出库移回可分配库区后，任务进入已完成。

ALTER TABLE warehouse_fulfillment.return_task
    ADD COLUMN fact_id text;

ALTER TABLE warehouse_fulfillment.return_task
    DROP CONSTRAINT return_task_status;

ALTER TABLE warehouse_fulfillment.return_task
    ADD CONSTRAINT return_task_status CHECK (status IN ('OPEN', 'DONE')),
    ADD CONSTRAINT return_task_fact CHECK (
        (status = 'OPEN' AND fact_id IS NULL)
        OR (status = 'DONE' AND fact_id IS NOT NULL AND btrim(fact_id) <> '')
    );
