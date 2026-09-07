-- 接受前财务控制采用结果长成逐项（ADR-0125，票 sa-preacceptance-policy-view/03）。
--
-- 0005 的 acceptance_financial_control 一行只装一个结论：settlement-accounting 自 ADR-0122 起按策略正文
-- 逐项执行、冻结与暴露可同时在场，逐项结果与共同通过条件在 PS 域里无处表达，过去靠消费适配器折成一格
-- ——事后只见一个 HELD，看不见同一请求还记了一笔信用暴露；结论受限时第一项占下的资金也随之从记录里消失。
--
-- 父表加 joint_pass_condition：结论是按哪个条件推出的。可空，且只在 NOT_APPLICABLE 时为 NULL——合同明确
-- 无控制那一支没有执行过任何一项，也就没有条件可言；已执行的结果必带条件。集合首发一值，放宽时只改
-- CHECK，不给既有行默认（同 PC 正文 0024 那条纪律，ADR-0115 决定三）。
--
-- 子表 acceptance_financial_control_item 逐项一行，键上带父行全部四维（租户+委托+版本+时点）再加控制种类：
-- 同一版策略内一种控制至多一项（PC 正文行级约束，SA 构造期同守），判断顺序另立唯一——两行抢一个顺序
-- 答不出该按哪条。外键回父行：没有父行的项是孤儿，读口按父行取最新时点再取它的项，孤儿永远读不到、
-- 与没记过分不开。
--
-- 形状照 NewControlItemResult：受限必带该项自己的原因（没有原因的业务限制说不出限制什么），成立不带
-- （成立的依据在提供方的占用记录本身，这里再带一份就是第二处定义）。全部 CHECK 走 IS NULL / IS NOT NULL
-- 缝（f822e1d 入册的三值逻辑纪律）。
--
-- ADD COLUMN 带 CHECK 而无默认：CI 走的是空库、本机门禁库每次是新库，没有一行既有数据要补；真有的话那些
-- 行也是本仓自己的合成 S，不是租户数据。

ALTER TABLE parcel_shipment.acceptance_financial_control
    ADD COLUMN joint_pass_condition text,
    ADD CONSTRAINT acceptance_financial_control_joint_pass_closed
        CHECK (joint_pass_condition IS NULL OR joint_pass_condition IN ('ALL_CONTROLS_PASS')),
    ADD CONSTRAINT acceptance_financial_control_joint_pass_shape
        CHECK (
            (control_outcome = 'NOT_APPLICABLE' AND joint_pass_condition IS NULL)
            OR (control_outcome <> 'NOT_APPLICABLE' AND joint_pass_condition IS NOT NULL)
        );

CREATE TABLE parcel_shipment.acceptance_financial_control_item (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,
    submission_version  text        NOT NULL,
    as_of_at            timestamptz NOT NULL,
    control_kind        text        NOT NULL,

    evaluation_order    integer     NOT NULL,
    item_conclusion     text        NOT NULL,
    basis_ref           text,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT acceptance_financial_control_item_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, submission_version, as_of_at, control_kind),

    CONSTRAINT acceptance_financial_control_item_order_unique
        UNIQUE (tenant_id, shipment_request_id, submission_version, as_of_at, evaluation_order),

    CONSTRAINT acceptance_financial_control_item_parent
        FOREIGN KEY (tenant_id, shipment_request_id, submission_version, as_of_at)
        REFERENCES parcel_shipment.acceptance_financial_control
            (tenant_id, shipment_request_id, submission_version, as_of_at),

    CONSTRAINT acceptance_financial_control_item_kind_closed
        CHECK (control_kind IN ('PREPAID_FREEZE', 'CREDIT_CHECK')),

    CONSTRAINT acceptance_financial_control_item_conclusion_closed
        CHECK (item_conclusion IN ('SATISFIED', 'RESTRICTED')),

    CONSTRAINT acceptance_financial_control_item_order_positive
        CHECK (evaluation_order > 0),

    CONSTRAINT acceptance_financial_control_item_shape
        CHECK (
            (item_conclusion = 'RESTRICTED' AND basis_ref IS NOT NULL AND btrim(basis_ref) <> '')
            OR (item_conclusion = 'SATISFIED' AND basis_ref IS NULL)
        )
);
