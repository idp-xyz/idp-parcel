-- 关联候选册与执行事实引用册。两张表都只承载「本上下文已经收到并接受的引用」，
-- 不复制也不代管别的上下文拥有的本体。
--
-- association_candidate：可供外部舱单引用关联的申报单元候选。关联只能基于承运商责任、
-- 监管程序、方向、适用时间和明确范围形成，同编号/同袋/同总单/同班次不自动证明同一
-- 对象（CONTEXT 硬句 148）——因此候选行携带的正是领域 Associate 逐维比对的那三维，
-- 而不是一个「看起来像」的模糊匹配分数。空清单是「没有能匹配的」如实答案，引用保持
-- 待关联；库里因此没有占位候选行。
--
-- execution_fact：节点、运输方或其他执行方提供的实际物理执行事实的**引用**。事实本体
-- 归执行方所有（CONTEXT-MAP `NO → CC`：节点提供实测、查验协作与处置执行事实；
-- node-operations CONTEXT「关务只引用节点结果，双方不得共同编辑承接决定或实际事实」）。
-- 本表因此只增不改：没有 UPDATE 路径，迟到、重复与更正各自成行，由核对按来源事实与
-- 适用时间重新形成判断，不按最后到达覆盖（CONTEXT 生命周期 304）。空清单是如实答案
-- ——决定推导不出执行，没有事实就是证据不足（领域折叠为`证据不足`）。

CREATE TABLE customs_compliance.association_candidate (
    tenant_id     text NOT NULL,
    unit_id       text NOT NULL,
    procedure_ref text NOT NULL,
    direction     text NOT NULL,
    scope_ref     text NOT NULL,

    CONSTRAINT association_candidate_pkey
        PRIMARY KEY (tenant_id, unit_id, procedure_ref, direction, scope_ref),

    CONSTRAINT association_candidate_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),
    CONSTRAINT association_candidate_direction_closed
        CHECK (direction IN ('IMPORT', 'EXPORT'))
);

CREATE TABLE customs_compliance.execution_fact (
    tenant_id    text        NOT NULL,
    decision_id  text        NOT NULL,
    fact_ref     text        NOT NULL,

    executor_ref text        NOT NULL,
    scope_ref    text        NOT NULL,
    units        integer     NOT NULL,
    occurred_at  timestamptz NOT NULL,

    CONSTRAINT execution_fact_pkey
        PRIMARY KEY (tenant_id, decision_id, fact_ref),

    CONSTRAINT execution_fact_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(executor_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),
    -- 领域 NewExecutionFact 要求 Units > 0：零件的「执行事实」证明不了任何执行范围，
    -- 部分执行有它自己的格（核对折出`部分覆盖`），不靠一条零数量事实表达。
    CONSTRAINT execution_fact_units_positive
        CHECK (units > 0)
);
