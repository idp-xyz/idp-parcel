-- 建账导入的批准记下所据的批准职责规则修订（UC-WF-011）。
-- 待批准与已驳回没有这一格。已入账可以还没有：直接入账的路径不经职责规则。
-- 本迁移不种任何行。

ALTER TABLE warehouse_fulfillment.stock_import
    ADD COLUMN duty_revision integer;

ALTER TABLE warehouse_fulfillment.stock_import
    DROP CONSTRAINT stock_import_status_shape;

ALTER TABLE warehouse_fulfillment.stock_import
    ADD CONSTRAINT stock_import_status_shape
        CHECK (
            (
                status = 'PENDING'
                AND poster IS NULL
                AND posted_at IS NULL
                AND goods_movement_id IS NULL
                AND carton_movement_id IS NULL
                AND rejector IS NULL
                AND rejected_at IS NULL
                AND rejection_note IS NULL
                AND duty_revision IS NULL
            )
            OR (
                status = 'POSTED'
                AND poster IS NOT NULL
                AND posted_at IS NOT NULL
                AND rejector IS NULL
                AND rejected_at IS NULL
                AND rejection_note IS NULL
                AND (goods_movement_id IS NOT NULL OR carton_movement_id IS NOT NULL)
                AND (duty_revision IS NULL OR duty_revision >= 1)
            )
            OR (
                status = 'REJECTED'
                AND rejector IS NOT NULL
                AND rejected_at IS NOT NULL
                AND poster IS NULL
                AND posted_at IS NULL
                AND goods_movement_id IS NULL
                AND carton_movement_id IS NULL
                AND duty_revision IS NULL
            )
        );
