-- 预占固定需求来源与判断时点，显式释放有自己的身份（票 demo-chain-gaps/20，ADR-0203 决定三至五）。
--
-- 0004 的预占行只有三量与有效期：说不出这份量从哪来，也说不出从哪一刻起占；已释放只是一个累计数，同一次释放重交
-- 一遍就再加一遍，分不出重放。预占与释放此前没有任何生产写入口（Reserve、Release 在 cmd 下零调用），表里应当没有
-- 行；有行就拒绝迁移，不替任何人补一个来源或一个判断时点。释放子表外键挂在预占上，预占表无行时它也无行。
--
-- 来源种类是封闭词，与领域 domain.CapacityDemandSourceKind 的常量逐词相同。单位不入列：池只有一个单位，声明的单位
-- 与它不同就不成预占或释放，存一份只会多一处可能不一致的格。
--
-- 已释放列留着：查阅册按它求和。它与释放子表逐行之和必须相等，跨表的这一条库上不钉，由领域重建门核。
--
-- 释放子表随所属预占删：池按值换存（Replace 先删本池全部预占再按新值插回），释放明细跟着预占一并换。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：本迁移新增的列都是 NOT NULL。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.capacity_reservation) THEN
        RAISE EXCEPTION USING MESSAGE =
            'capacity_reservation 已有行，不替它们补需求来源与判断时点：请导出这些预占备查，由作出预占的一方经预占口'
            || '重新声明后清空本表，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE transport_fulfillment.capacity_reservation
    ADD COLUMN source_kind text        NOT NULL,
    ADD COLUMN source_ref  text        NOT NULL,
    ADD COLUMN reserved_at timestamptz NOT NULL;

ALTER TABLE transport_fulfillment.capacity_reservation
    ADD CONSTRAINT capacity_reservation_source_kind_closed
        CHECK (source_kind IN ('OPERATOR_DECLARATION', 'PREPARATION_DEMAND_SNAPSHOT'));

ALTER TABLE transport_fulfillment.capacity_reservation
    ADD CONSTRAINT capacity_reservation_source_ref_not_blank
        CHECK (btrim(source_ref) <> '');

ALTER TABLE transport_fulfillment.capacity_reservation
    ADD CONSTRAINT capacity_reservation_window_after_instant
        CHECK (valid_until > reserved_at);

CREATE TABLE transport_fulfillment.capacity_release (
    tenant_id      text        NOT NULL,
    pool_id        text        NOT NULL,
    release_id     text        NOT NULL,

    reservation_id text        NOT NULL,
    quantity       bigint      NOT NULL,
    source_kind    text        NOT NULL,
    source_ref     text        NOT NULL,
    released_at    timestamptz NOT NULL,

    -- 释放引用在池内唯一，不只在一份预占内唯一：同一引用挪到另一份预占上是另一个决定。
    CONSTRAINT capacity_release_pkey
        PRIMARY KEY (tenant_id, pool_id, release_id),

    CONSTRAINT capacity_release_reservation_fkey
        FOREIGN KEY (tenant_id, pool_id, reservation_id)
        REFERENCES transport_fulfillment.capacity_reservation (tenant_id, pool_id, reservation_id)
        ON DELETE CASCADE,

    CONSTRAINT capacity_release_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pool_id) <> ''
            AND btrim(release_id) <> ''
            AND btrim(reservation_id) <> ''
            AND btrim(source_ref) <> ''
        ),

    CONSTRAINT capacity_release_quantity_positive
        CHECK (quantity > 0),

    CONSTRAINT capacity_release_source_kind_closed
        CHECK (source_kind IN ('OPERATOR_DECLARATION', 'PREPARATION_DEMAND_SNAPSHOT'))
);
