-- 仓测尺寸重量（UC-WF-002；仓配履约 CONTEXT「仓测尺寸重量」「主数据登记」）。
-- 按（商品，包装层级）或按在库的一箱只追加版本，绝不 UPDATE 或 DELETE。
--
-- 「仓须是本租户已登记的自营仓」「商品已登记且包装层级已声明」「箱在此刻在这个仓在库」「单位在内置目录且类别对」由应用层判；
-- 库上守四件事：同一事实身份至多一版（主键）；设备序号为正；长宽高与重量为正；对象形状在 GOODS_LEVEL 与 CARTON 之间互斥自洽。
-- 本表不对仓、商品或整箱账建外键。当前那一版按作业事实发生次序（occurred_at, device_sequence, received_at, fact_id）派生，不存成可改写状态。
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。

CREATE TABLE warehouse_fulfillment.warehouse_measurement (
    tenant_id        text          NOT NULL,
    fact_id          text          NOT NULL,
    device_id        text          NOT NULL,
    operator_id      text          NOT NULL,
    occurred_at      timestamptz   NOT NULL,
    device_sequence  bigint        NOT NULL,
    received_at      timestamptz   NOT NULL,
    warehouse_code   text          NOT NULL,

    target_kind      text          NOT NULL,
    owner_account_id text          NOT NULL,
    goods_code       text,
    packaging_level  text,
    carton_number    text,

    length_value     numeric(15,6) NOT NULL,
    width_value      numeric(15,6) NOT NULL,
    height_value     numeric(15,6) NOT NULL,
    dimensions_unit  text          NOT NULL,
    weight_value     numeric(15,6) NOT NULL,
    weight_unit      text          NOT NULL,

    CONSTRAINT warehouse_measurement_pkey
        PRIMARY KEY (tenant_id, fact_id),

    CONSTRAINT warehouse_measurement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_id) <> ''
            AND btrim(device_id) <> ''
            AND btrim(operator_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(dimensions_unit) <> ''
            AND btrim(weight_unit) <> ''
        ),

    CONSTRAINT warehouse_measurement_sequence_positive
        CHECK (device_sequence >= 1),

    CONSTRAINT warehouse_measurement_values_positive
        CHECK (
            length_value > 0
            AND width_value > 0
            AND height_value > 0
            AND weight_value > 0
        ),

    CONSTRAINT warehouse_measurement_target_shape
        CHECK (
            (
                target_kind = 'GOODS_LEVEL'
                AND goods_code IS NOT NULL AND btrim(goods_code) <> ''
                AND packaging_level IS NOT NULL AND btrim(packaging_level) <> ''
                AND carton_number IS NULL
            )
            OR (
                target_kind = 'CARTON'
                AND goods_code IS NULL
                AND packaging_level IS NULL
                AND carton_number IS NOT NULL AND btrim(carton_number) <> ''
            )
        )
);

CREATE INDEX warehouse_measurement_by_goods_level
    ON warehouse_fulfillment.warehouse_measurement
        (tenant_id, owner_account_id, goods_code, packaging_level, occurred_at, device_sequence, received_at, fact_id)
    WHERE target_kind = 'GOODS_LEVEL';

CREATE INDEX warehouse_measurement_by_carton
    ON warehouse_fulfillment.warehouse_measurement
        (tenant_id, owner_account_id, warehouse_code, carton_number, occurred_at, device_sequence, received_at, fact_id)
    WHERE target_kind = 'CARTON';
