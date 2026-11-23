-- 开放拣货任务按格占用。别名记在任务上，才能和库存格对齐；没贴标保持 NULL。

ALTER TABLE warehouse_fulfillment.pick_task
    ADD COLUMN label_alias text,
    ADD CONSTRAINT pick_task_alias_not_blank CHECK (label_alias IS NULL OR btrim(label_alias) <> '');
