-- 取消先后冲突册（ADR-0254 决定二第 3 格；票 docs-consistency/code/35）。
--
-- 取消已提交之后才到的边界事实（网络服务被采用的收寄来源、面单渠道服务的实际承运商首次有效收寄），发生
-- 不晚于取消的业务时间，是事实冲突：不形成责任起点也不形成非取消终局，已提交的取消不被改写。冲突重投推不动，
-- 消费门入账；入账之前冲突先落在这里，按包裹查得出、交人，解开它的业务决定日后从这里取。
--
-- 只追加。主键是（租户，包裹，边界种类，边界那一代，取消）：同一来源重投判出的是同一份冲突，撞主键即已有记录；
-- 来源更正换了一代就是另一次比较，另起一行。
CREATE TABLE parcel_shipment.cancellation_order_conflict (
    tenant_id        text        NOT NULL,
    parcel_id        text        NOT NULL,
    boundary_kind    text        NOT NULL,
    boundary_ref     text        NOT NULL,
    cancellation_id  text        NOT NULL,

    cancelled_at     timestamptz NOT NULL,
    boundary_at      timestamptz NOT NULL,
    detected_at      timestamptz NOT NULL,

    CONSTRAINT cancellation_order_conflict_pkey
        PRIMARY KEY (tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id),

    CONSTRAINT cancellation_order_conflict_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(boundary_ref) <> ''
            AND btrim(cancellation_id) <> ''
        ),

    CONSTRAINT cancellation_order_conflict_kind_closed
        CHECK (boundary_kind IN ('NETWORK_INTAKE', 'CARRIER_FIRST_EFFECTIVE_PICKUP')),

    -- 冲突的定义本身：边界事实发生不晚于取消的业务时间（相等按冲突）。晚于取消的是取消在先，不是冲突，
    -- 落进这张表就是判法之外另有人写了它。
    CONSTRAINT cancellation_order_conflict_boundary_not_after_cancellation
        CHECK (boundary_at <= cancelled_at)
);

-- 按包裹查冲突（CancellationOrderConflictRegister.ListByParcel）由主键前缀承担，不另建索引。
