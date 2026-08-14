-- 费用确认条件的两张表：目录说「这类费用要什么依据」，依据登记说「这笔费用到了什么
-- 依据」，核对是两者的交集。
--
-- 分两张而不是一张，是为了让「确认条件已满足」没有第三条成立路径。合成一张，行上就会
-- 出现一个可直接写成`已满足`的字段，而 UC-SA-002 的`费用已确认`要求确认条件真的满足
-- 过——一个可配置的布尔值正是`默认转正`换了个存放位置。
--
-- 目录内容（哪类费用要哪种依据）属实例半边：没有租户时表是空的，视图据以答`未配置`，
-- 编排保持未决。空表与「配了但依据没到」是两种答案，不共用一格。

CREATE TABLE settlement_accounting.charge_confirmation_condition (
    tenant_id           text        NOT NULL,
    fee_item            text        NOT NULL,

    required_basis_kind text        NOT NULL,
    registered_at       timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT charge_confirmation_condition_pkey
        PRIMARY KEY (tenant_id, fee_item),

    CONSTRAINT charge_confirmation_condition_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(required_basis_kind) <> ''
        )
);

-- 依据登记按（费用+费用项目+依据种类）：一笔费用可以到达多种依据，核对只认目录点名
-- 的那一种。种类进主键而不是只存一列值，是因为「到了另一种依据」不该顶替要求的那种。
CREATE TABLE settlement_accounting.charge_confirmation_basis (
    tenant_id    text        NOT NULL,
    charge_id    text        NOT NULL,
    fee_item     text        NOT NULL,
    basis_kind   text        NOT NULL,

    basis_ref    text        NOT NULL,
    recorded_at  timestamptz NOT NULL,
    inserted_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT charge_confirmation_basis_pkey
        PRIMARY KEY (tenant_id, charge_id, fee_item, basis_kind),

    CONSTRAINT charge_confirmation_basis_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(charge_id) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(basis_kind) <> ''
            AND btrim(basis_ref) <> ''
        )
);
