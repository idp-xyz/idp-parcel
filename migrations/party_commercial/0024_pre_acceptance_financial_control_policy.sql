-- 接受前财务控制策略册（票 party-commercial-context-gaps/07，ADR-0115）：一个策略版本的正文——
-- 要执行的控制项集合与它们的共同通过条件。
--
-- 与版本册分表，判据同 0020/0023：版本回答「有没有这份策略对象」，正文回答「控制怎么做」。父子两表
-- 照 0014/0023：父行一条持共同通过条件，子行按控制项成行。
--
-- **正文只登要执行的控制项，没有「明确无控制」那一格。** 「无控制」由客户合同版本声明并带不适用依据
-- ——版本级在 0007，按费用范围在 0012 的绑定表。策略正文若有这一格，一个范围就能经 0012 的
-- policy_id 指名它而不给 inapplicability_basis，那正是 CONTEXT 明禁的「用缺失结果或默认通过代替」。
-- 所以 control_kind 的 CHECK 只有两值，且它是子表主键的一部分：「不得硬编码为永久互斥的三选一枚举」
-- 在这里的落法是一版策略可以有多行，集合以迁移放宽 CHECK 而扩。
--
-- 子行两个唯一性各守一句 CONTEXT：主键含（control_kind, charge_scope_ref）守「同一范围上同一种控制至多
-- 一行」，evaluation_order 的 UNIQUE 守「判断顺序」——两行抢一个顺序答不出该先做哪个。至少一项
-- （ADR-0115 Decision 一）：SQL 表达不了「父行至少一子行」，有父行而零子行是坏数据——装载走 error，
-- 不得折成未配置；无父行才是 found=false。
--
-- joint_pass_condition 首发只认 ALL_CONTROLS_PASS 却仍然成列、NOT NULL：列在，这个条件就是租户说出来
-- 的而不是产品替它默认的；将来放宽只改 CHECK，既有行一字不动。没有这一列，放宽那天要么给既有行一个
-- 默认值、要么把空读成「全部通过」，两条都是隐含默认。
--
-- failure_disposition 两值逐字对应 UC-PS-001「任一必需控制不通过时按策略拒绝或进入授权处置」。它只答
-- 委托的去向，不拥有拒绝决定（归 parcel-shipment）也不拥有控制结果（归 settlement-accounting）。
--
-- charge_scope_ref 与 responsibility_ref 都是引用不是封闭集：费用范围与 0012 绑定表、结算政策用同一
-- 词汇，消费方拿委托的费用范围一路对下来；责任方是谁属实例半边。两者都不设外键。
--
-- 表上刻意没有信用政策版本、结算账户、价格依据、金额与阈值——各有所有者（0020、PAR-SET-01/02、
-- settlement-accounting 的结果），写进来就是同一件事第二处定义；也没有合同引用（合同 → 策略由 0012
-- 拥有）与有效区间（在版本壳上，照 0023 不抄第二份）。
--
-- object_kind CHECK = 5（PreAcceptanceFinancialControlPolicyObject）。
--
-- 本表族不进整册装载（LoadForScope）、不进 ViewRevision：解析按范围与锚点选版本壳，正文由消费方按
-- 已选中的版本点读（ports.PreAcceptanceFinancialControlPolicyContentView）。settlement-accounting 今天
-- 的 LoadControlPolicy 不读本表，改读另立票。
--
-- 行属实例半边：哪几项控制、什么顺序、失败去哪由租户登记，今天没有租户因而本表族为空。表上没有任何
-- 默认控制——缺正文就是未登记。

CREATE TABLE party_commercial.pre_acceptance_financial_control_policy (
    tenant_id             text        NOT NULL,
    object_kind           smallint    NOT NULL,
    object_id             text        NOT NULL,
    version_label         text        NOT NULL,

    joint_pass_condition  text        NOT NULL,
    registered_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pre_acceptance_financial_control_policy_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT pre_acceptance_financial_control_policy_version_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.commercial_version
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT pre_acceptance_financial_control_policy_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    CONSTRAINT pre_acceptance_financial_control_policy_kind_only
        CHECK (object_kind = 5),

    -- 镜像 domain.JointPassCondition 的封闭集，首发一值。
    CONSTRAINT pre_acceptance_financial_control_policy_joint_pass_closed
        CHECK (joint_pass_condition IN ('ALL_CONTROLS_PASS'))
);

-- 控制项：一行一项。主键含种类与范围，UNIQUE 含顺序，两个唯一性各守一句 CONTEXT（见头注）。
CREATE TABLE party_commercial.pre_acceptance_financial_control_item (
    tenant_id            text     NOT NULL,
    object_kind          smallint NOT NULL,
    object_id            text     NOT NULL,
    version_label        text     NOT NULL,
    control_kind         text     NOT NULL,
    charge_scope_ref     text     NOT NULL,
    evaluation_order     integer  NOT NULL,
    failure_disposition  text     NOT NULL,
    responsibility_ref   text     NOT NULL,

    CONSTRAINT pre_acceptance_financial_control_item_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, control_kind, charge_scope_ref),

    CONSTRAINT pre_acceptance_financial_control_item_order_unique
        UNIQUE (tenant_id, object_kind, object_id, version_label, evaluation_order),

    CONSTRAINT pre_acceptance_financial_control_item_policy_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.pre_acceptance_financial_control_policy
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    -- 镜像 domain.PreAcceptanceControlKind：两值，没有「无控制」。
    CONSTRAINT pre_acceptance_financial_control_item_kind_closed
        CHECK (control_kind IN ('PREPAID_FREEZE', 'CREDIT_CHECK')),

    -- 镜像 domain.ControlFailureDisposition：两值，逐字对应 UC-PS-001。
    CONSTRAINT pre_acceptance_financial_control_item_disposition_closed
        CHECK (failure_disposition IN ('REJECT', 'AUTHORIZED_DISPOSITION')),

    CONSTRAINT pre_acceptance_financial_control_item_not_blank
        CHECK (btrim(charge_scope_ref) <> '' AND btrim(responsibility_ref) <> ''),

    -- 顺序从 1 起数：零与负数不是「排第几」的答案（domain.NewPreAcceptanceControlItem 同判据）。
    CONSTRAINT pre_acceptance_financial_control_item_order_positive
        CHECK (evaluation_order > 0)
);
