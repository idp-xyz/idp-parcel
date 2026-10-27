-- 取消先后冲突的解开决定与被取消挡住的来源回放（ADR-0295；票 docs-consistency/code/76）。
--
-- cancellation_order_conflict_decision 记下人对册上一份冲突（0029）做过的决定：谁、哪一种、凭什么、何时。只追加：冲突行不改，
-- 决定行也不改不删——一份冲突是否「未解开」由有没有对应的决定行决定，不在冲突行上加状态列。主键就是冲突行的主键（租户，包裹，
-- 边界种类，边界那一代，取消），一份冲突至多被决定一次；两人同时决定，主键相撞，只有一笔成立，败者读回赢家。外键指向冲突行，
-- 决定不会挂在一份册上没有的冲突上。
--
-- decision_kind 是封闭两值：CANCELLATION_NOT_UPHELD（认定取消依据不成立，那份取消不再是有效取消）与 BOUNDARY_FACT_REFERRED
-- （认定这一代边界事实有误，交来源方更正，取消照旧）。decided_by 是（发行方，sub）这一对的引用，decision_basis 是决定人核对后
-- 留下的依据，两样必填。
--
-- cancellation_order_conflict_unresolved 服务运营查阅的 ORDER BY detected_at LIMIT：主键前缀按（租户，包裹）排，按租户取最早
-- 发现的未解开冲突用不上它。
--
-- cancellation_blocked_replay 记下被一份取消挡住的来源回放所需的原料：收寄采用判出冲突时的来源规格，终局采用因取消不采用时的
-- 责任结果规格。取消被认定不成立之后，续办凭它把当时被挡住的来源重新交给原来的编排，不必回头找来源方重投。只追加；主键
-- （租户，包裹，取消，链，来源）让同一来源重投不翻倍。replay 是封闭字段的 JSON，由适配器读写、领域构造器复验。
--
-- 全部 CHECK 走 IS NULL / IS NOT NULL 缝（f822e1d 入册的三值逻辑纪律）；本两表的列都 NOT NULL，没有三值问题。

CREATE TABLE parcel_shipment.cancellation_order_conflict_decision (
    tenant_id       text        NOT NULL,
    parcel_id       text        NOT NULL,
    boundary_kind   text        NOT NULL,
    boundary_ref    text        NOT NULL,
    cancellation_id text        NOT NULL,

    decision_kind   text        NOT NULL,
    decided_by      text        NOT NULL,
    decision_basis  text        NOT NULL,
    decided_at      timestamptz NOT NULL,

    CONSTRAINT cancellation_order_conflict_decision_pkey
        PRIMARY KEY (tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id),

    CONSTRAINT cancellation_order_conflict_decision_conflict_fkey
        FOREIGN KEY (tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id)
        REFERENCES parcel_shipment.cancellation_order_conflict
            (tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id),

    CONSTRAINT cancellation_order_conflict_decision_kind_closed
        CHECK (decision_kind IN ('CANCELLATION_NOT_UPHELD', 'BOUNDARY_FACT_REFERRED')),

    CONSTRAINT cancellation_order_conflict_decision_not_blank
        CHECK (
            btrim(decided_by) <> ''
            AND btrim(decision_basis) <> ''
        )
);

-- 有效取消读（ParcelCancellations.FindCancellation）按（租户，包裹，取消）排除被认定不成立的取消；主键前缀（租户，包裹）承担
-- 定位，这条部分索引只收「不成立」的那一种，让读取消的热路径不扫别种决定。
CREATE INDEX cancellation_order_conflict_decision_not_upheld
    ON parcel_shipment.cancellation_order_conflict_decision (tenant_id, parcel_id, cancellation_id)
    WHERE decision_kind = 'CANCELLATION_NOT_UPHELD';

CREATE TABLE parcel_shipment.cancellation_blocked_replay (
    tenant_id       text        NOT NULL,
    parcel_id       text        NOT NULL,
    cancellation_id text        NOT NULL,
    chain           text        NOT NULL,
    source_ref      text        NOT NULL,

    replay          text        NOT NULL,
    recorded_at     timestamptz NOT NULL,

    CONSTRAINT cancellation_blocked_replay_pkey
        PRIMARY KEY (tenant_id, parcel_id, cancellation_id, chain, source_ref),

    CONSTRAINT cancellation_blocked_replay_chain_closed
        CHECK (chain IN ('NETWORK_INTAKE_ADOPTION', 'PARCEL_FINAL_ADOPTION')),

    CONSTRAINT cancellation_blocked_replay_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(cancellation_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(replay) <> ''
        )
);
