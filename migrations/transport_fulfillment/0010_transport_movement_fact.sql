-- 实际移动事实（tf-unwired-seven/06）。
--
-- **主键带版本，写入只插不改。** 迟到与更正形成新版本、原记录不被改写。CONTEXT 那句
-- 「协作事项取消、替代或缩小范围时……已经形成的权威交接、实际移动和费用责任继续保留」
-- 正是靠这一点守住的：上游意图取消抹不掉一条已发生的事实。把版本挪出主键就等于允许
-- UPDATE 回写一条已发生的移动。
--
-- **没有任何控制或交接列。** 移动不等于交接、不结束控制——控制转移只随权威交接或有效交付
-- 成立。CONTEXT 逐项列举过扫描、车辆到场、物理装载都不能替代控制边界，少这几列是有意的。
--
-- **也没有履约段外键。** 移动发生在某个段的控制期内，但两者是引用关系不是状态机（移动不
-- 结束段，中断也不结束）。加一条外键会让「段必须先在」成为写入前提，而事实可能先于段的
-- 登记到达——那时正确的做法是照实记下事实，不是拒收它。
CREATE TABLE transport_fulfillment.transport_movement_fact (
    tenant_id        text        NOT NULL,
    fact_ref         text        NOT NULL,
    version          text        NOT NULL,

    schedule_ref     text        NOT NULL,
    kind             text        NOT NULL,
    location_ref     text        NOT NULL,
    source_ref       text        NOT NULL,
    occurred_at      timestamptz NOT NULL,

    -- 门禁放行引用可空，而那个空是**有依据的缺席不是数据缺失**：CONTEXT 说门禁出发可阻断，
    -- 但不是每次移动都经门禁。用 NULL 不用空串，正是为了让这两件事分得开——下一个人看到
    -- NULL 该读作「这一次没有门禁约束」，不是「这一行没填完」。
    gate_clearance   text,

    corrects_version text,
    corrected_at     timestamptz,

    recorded_at      timestamptz NOT NULL,

    CONSTRAINT transport_movement_fact_pkey
        PRIMARY KEY (tenant_id, fact_ref, version),

    CONSTRAINT transport_movement_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(schedule_ref) <> ''
            AND btrim(location_ref) <> ''
            AND btrim(source_ref) <> ''
        ),

    -- 事实种类封闭三值。中断、折返、改降属执行结果家族，随履约段与班次结果另行表达——
    -- 这里没有它们的格是有意的，多一格就等于让两个家族共用一个可覆盖状态。
    CONSTRAINT transport_movement_fact_kind_closed
        CHECK (kind IN ('DEPARTURE', 'IN_TRANSIT', 'ARRIVAL')),

    -- **门禁只约束装载出发。** 把放行依据挂到移动或到达上等于造了一个不存在的门；领域构造门
    -- 已拒，库面镜像它，因为绕过构造门的写入路径同样不该落进这种行。
    CONSTRAINT transport_movement_fact_gate_only_on_departure
        CHECK (gate_clearance IS NULL OR kind = 'DEPARTURE'),

    CONSTRAINT transport_movement_fact_gate_not_blank
        CHECK (gate_clearance IS NULL OR btrim(gate_clearance) <> ''),

    -- 更正两件成对：半截会装回一个「更正过但不知何时更正的」版本，那是领域任何路径都产不出
    -- 的状态。
    CONSTRAINT transport_movement_fact_correction_coupled
        CHECK ((corrects_version IS NOT NULL) = (corrected_at IS NOT NULL)),

    CONSTRAINT transport_movement_fact_corrects_not_blank
        CHECK (corrects_version IS NULL OR btrim(corrects_version) <> ''),

    -- 前版引用指向自己就是一条读不动的链。
    CONSTRAINT transport_movement_fact_corrects_not_self
        CHECK (corrects_version IS NULL OR corrects_version <> version),

    -- 更正晚于事实发生。更正的是对同一件已发生的事的记述，不是把它挪到更早。
    CONSTRAINT transport_movement_fact_corrected_after_occurred
        CHECK (corrected_at IS NULL OR corrected_at >= occurred_at)
);

-- 一条**故意没有下沉**的不变量：受监管门禁约束的出发在没有放行依据时立不成出发
-- （`ErrDepartureGateBlocked`）。
--
-- 库面看不见「这一次受不受门禁约束」——`GateRequired` 是判断的输入而不是事实的属性，它不进表：
-- 一条已经成立的出发要么带着放行依据、要么本来就不受门禁管，两者在库里的区别就是那一列有没有
-- 值。把 `gate_required` 存下来等于把判断过程当成事实存起来，而下一次规则变了那一列就开始说谎。
-- 所以这条只在领域守，本表只管「放行依据只能挂在出发上」。
