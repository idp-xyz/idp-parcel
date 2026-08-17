-- 接受判断任务的判断库：`UC-PS-001`「建立或续办独立接受判断任务并追加判断与处理尝试」，
-- 承载 AcceptanceJudgmentRecorder 的四个写口与 RecordedJudgmentReader 的读口。
--
-- 四张表而不是一张：可达性按成员逐条形成，财务控制作用在整份委托上，所采用商业解析每份
-- 委托只有当前一个，处理尝试只累积不参与决定——把它们塞进一张宽表，四种在场件矩阵会挤成
-- 一条谁也读不懂的 CHECK，而每加一类判断都要改所有行的形状。
--
-- 判断与处理尝试**只追加不覆盖**（`UC-PS-001` 所有权一节要求保留「每项……判断的版本与
-- 依据」）。当前采用的那一份由读口按判断时点取最新，而不是靠一个 is_current 列：翻旧插新
-- 要两条语句，而重放一份更早的判断会先把当前那行翻掉再撞键什么也没插——库里就此一行当前
-- 判断都不剩。时点是权威回显过的业务时刻，重判必须以新时点发起（`AT-PS-037`），因此它就是
-- 采用顺序本身。
--
-- 所采用商业解析是唯一的例外，按端口原文「记入不同标识时以后写的为准」写成每份委托一行的
-- 覆盖：不覆盖的话，提交决定前的重解会对着已被取代的那次依据做。
--
-- 没有 submission_version 列，因为端口的四个写口与读口都不携带它。多版本之后旧版本的判断
-- 会被读进新版本的决定，这是 ADR-0045 明写的已知后续项（「读口要长出版本维度……随编排切片
-- 一起收」），本迁移不替它定形状。
--
-- 不设指向 shipment_request 的外键。推进判断的编排（advance_acceptance_judgment）从头到尾
-- 不读也不写委托聚合，判断记录与聚合快照因而不在同一条写入路径上；外键会让一次判断记录取决
-- 于另一条路径的落库时机。同模块的 intake_adoption / parcel_cancellation / final_outcome
-- 携带 shipment_request_id 时同样只存引用。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL /
-- IS NOT NULL，没有一条比较式能单独以 NULL 决定约束。

CREATE TABLE parcel_shipment.acceptance_reachability_judgment (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,
    parcel_id           text        NOT NULL,
    as_of_at            timestamptz NOT NULL,

    judgment_id         text,
    judgment_value      text        NOT NULL,
    basis_ref           text,
    as_of_kind          text        NOT NULL,
    as_of_semantics     text        NOT NULL,
    as_of_policy        text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    -- 主键带时点：同一成员的每一次重判各占一行，同成员同时点重复到达是重放。租户在最前，
    -- 委托标识的唯一性本就按租户圈定（ADR-0003）。
    CONSTRAINT acceptance_reachability_judgment_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, parcel_id, as_of_at),

    CONSTRAINT acceptance_reachability_judgment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(as_of_semantics) <> ''
            AND btrim(as_of_policy) <> ''
        ),

    -- 三值判断加`不适用`，封闭四行（UC-NR-002）：`不适用`不是第五种可达，也不能顶替三值
    -- 里的任何一个，所以它在库里占自己那一格。
    CONSTRAINT acceptance_reachability_judgment_value_closed
        CHECK (judgment_value IN
            ('REACHABLE', 'UNREACHABLE', 'INSUFFICIENT_EVIDENCE', 'NOT_APPLICABLE')),

    CONSTRAINT acceptance_reachability_judgment_as_of_kind_closed
        CHECK (as_of_kind IN ('REACHABILITY', 'FINANCIAL_CONTROL')),

    -- 与 NewReachabilityJudgment 逐条对上：`不适用`必带依据、不要求标识（那一支下权威不形成
    -- 判断，也就没有标识可引用），其余三值必带标识。不多要一条——领域没有禁止三值携带依据，
    -- 库里禁了就会拒掉一份领域认可的判断。
    CONSTRAINT acceptance_reachability_judgment_shape
        CHECK (
            (judgment_value = 'NOT_APPLICABLE'
                AND basis_ref IS NOT NULL AND btrim(basis_ref) <> '')
            OR (judgment_value <> 'NOT_APPLICABLE'
                AND judgment_id IS NOT NULL AND btrim(judgment_id) <> '')
        )
);

CREATE TABLE parcel_shipment.acceptance_financial_control (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,
    as_of_at            timestamptz NOT NULL,

    result_id           text,
    control_outcome     text        NOT NULL,
    basis_ref           text,
    as_of_kind          text        NOT NULL,
    as_of_semantics     text        NOT NULL,
    as_of_policy        text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    -- 不按成员分行：控制作用在整份委托上，逐成员一行会把一份委托的资金占用记成成员份数。
    CONSTRAINT acceptance_financial_control_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, as_of_at),

    CONSTRAINT acceptance_financial_control_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(as_of_semantics) <> ''
            AND btrim(as_of_policy) <> ''
        ),

    -- 封闭四值，且没有「视同通过」那一格：一个表示「没控制成但先过」的取值就是默认放行的
    -- 载体，而控制没能形成时编排保持未决、根本不写这张表。
    CONSTRAINT acceptance_financial_control_outcome_closed
        CHECK (control_outcome IN
            ('HELD', 'RESTRICTED', 'NOT_APPLICABLE', 'CREDIT_EXPOSED')),

    CONSTRAINT acceptance_financial_control_as_of_kind_closed
        CHECK (as_of_kind IN ('REACHABILITY', 'FINANCIAL_CONTROL')),

    -- 与 NewFinancialControlResult 逐条对上：`已冻结`与`信用暴露已记录`是两种执行通过，依据
    -- 在占用记录本身；其余必带依据——没有依据的`明确无控制`与「默认信用通过」无从分辨。
    -- `明确无控制`那一支提供方不形成冻结，因而不要求结果标识。
    CONSTRAINT acceptance_financial_control_shape
        CHECK (
            (control_outcome IN ('HELD', 'CREDIT_EXPOSED')
                OR (basis_ref IS NOT NULL AND btrim(basis_ref) <> ''))
            AND (control_outcome = 'NOT_APPLICABLE'
                OR (result_id IS NOT NULL AND btrim(result_id) <> ''))
        )
);

-- 本轮判断所采用的那次商业解析。每份委托一行、后写覆盖：多轮判断本就该采用同一次解析，
-- 而决定期的提交前重解会换一次新的，留着旧的会让下一轮对着已被取代的依据重校。
CREATE TABLE parcel_shipment.acceptance_adopted_resolution (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,

    resolution_id       text        NOT NULL,
    adopted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT acceptance_adopted_resolution_pkey
        PRIMARY KEY (tenant_id, shipment_request_id),

    CONSTRAINT acceptance_adopted_resolution_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(resolution_id) <> ''
        )
);

-- 没能推进的那一轮。只留成功的判断，一份卡了十轮的委托看起来会和刚建单的一模一样。
CREATE TABLE parcel_shipment.acceptance_processing_attempt (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,
    continuation_ref    text        NOT NULL,
    attempted_at        timestamptz NOT NULL,

    reason_ref          text        NOT NULL,
    resume_path         text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    -- 续办引用由「未决原因 + 范围」派生，因此同一原因同一时刻重复到达是同一次尝试的重放；
    -- 真正卡了两轮会有两个时刻，各占一行。
    CONSTRAINT acceptance_processing_attempt_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, continuation_ref, attempted_at),

    CONSTRAINT acceptance_processing_attempt_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(continuation_ref) <> ''
            AND btrim(reason_ref) <> ''
        ),

    -- 三个等待态一一对应 CONTEXT 的续办路径：续办方分别是客户、系统与授权复核角色，多一格
    -- 少一格都会让催办催错人。
    CONSTRAINT acceptance_processing_attempt_resume_path_closed
        CHECK (resume_path IN ('CUSTOMER_SUPPLEMENT', 'INTERNAL_RETRY', 'MANUAL_REVIEW'))
);
