-- 容量池按（班次，维度）立身份；周期安排可逐维声明容量，按日期物化时随班次建池（票 demo-chain-gaps/17，ADR-0201
-- 决定一、三）。
--
-- 0004 的池只有单位没有维度。维度是准备评估把需求量配到池上的那一格（ADR-0201 决定八），单位推不出维度——
-- 同一个单位串在不同租户手里可以是不同的量。那张表从未有过生产写入口，应当没有行；有行就拒绝迁移，不替任何人
-- 把一个单位猜成维度。预占子表外键指向池，池表无行时它也无行，一并在这一道门里。
--
-- 池身份由（班次，维度）派生，写作「班次/维度」：一个班次在一个维度上只有一个池（ADR-0201 补记「池身份」）。
-- 库上用 CHECK 钉住派生关系，主键（租户，池）于是同时挡住同一班次同一维度的第二个池。维度是封闭词，与领域
-- domain.CapacityDimension 的常量逐词相同。
--
-- 周期安排的容量声明另立子表，按（租户，代码，版本，维度）一行，随所属那一版只插不改：一版周期安排的正文在它
-- 落册那一刻就定了，换声明就是下一版。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：本迁移新增的列都是 NOT NULL。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.capacity_pool) THEN
        RAISE EXCEPTION USING MESSAGE =
            'capacity_pool 已有行，不把单位猜成容量维度：请导出这些池与其预占备查，由登记它们的一方按班次与维度'
            || '重新登记后清空预占子表与本表，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE transport_fulfillment.capacity_pool
    ADD COLUMN dimension text NOT NULL;

ALTER TABLE transport_fulfillment.capacity_pool
    ADD CONSTRAINT capacity_pool_dimension_closed
        CHECK (dimension IN ('WEIGHT', 'VOLUME', 'PIECES', 'PALLET_POSITIONS', 'BAG_POSITIONS', 'CARGO_SLOTS'));

ALTER TABLE transport_fulfillment.capacity_pool
    ADD CONSTRAINT capacity_pool_identity_derived
        CHECK (pool_id = schedule_id || '/' || dimension);

CREATE TABLE transport_fulfillment.schedule_template_capacity (
    tenant_id        text    NOT NULL,
    template_code    text    NOT NULL,
    template_version integer NOT NULL,
    dimension        text    NOT NULL,

    unit_ref         text    NOT NULL,
    capacity         bigint  NOT NULL,

    CONSTRAINT schedule_template_capacity_pkey
        PRIMARY KEY (tenant_id, template_code, template_version, dimension),

    CONSTRAINT schedule_template_capacity_template_fkey
        FOREIGN KEY (tenant_id, template_code, template_version)
        REFERENCES transport_fulfillment.schedule_template (tenant_id, template_code, template_version),

    CONSTRAINT schedule_template_capacity_unit_not_blank
        CHECK (btrim(unit_ref) <> ''),

    CONSTRAINT schedule_template_capacity_dimension_closed
        CHECK (dimension IN ('WEIGHT', 'VOLUME', 'PIECES', 'PALLET_POSITIONS', 'BAG_POSITIONS', 'CARGO_SLOTS')),

    CONSTRAINT schedule_template_capacity_positive
        CHECK (capacity > 0)
);
