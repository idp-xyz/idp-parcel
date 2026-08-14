-- 分摊规则适用登记：来源金额身份 → 采用的分摊规则版本（UC-SA-006 步骤 2）。
--
-- 表里只有版本引用，没有分摊范围、权重依据或尾差处理——规则内容归声明它的那一侧
-- （`PAR-SET-06`），这里登的是「这条来源用哪一版」。把内容抄进来就是第二处定义，
-- 而两处一旦不一致，分出去的钱按哪份算都说不清。
--
-- 无行即`未配置`：视图答 found=false，编排保持来源金额未分摊。它绝不退化成默认比例、
-- 默认分母或平均分摊——UC-SA-006 的排除项把这三样点了名（AT-SA-123）。没有租户时
-- 表是空的，那正是首发唯一走得到的真实分支。

CREATE TABLE settlement_accounting.allocation_rule_applicability (
    tenant_id     text        NOT NULL,
    source_ref    text        NOT NULL,

    rule_version  text        NOT NULL,
    registered_at timestamptz NOT NULL,
    inserted_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT allocation_rule_applicability_pkey
        PRIMARY KEY (tenant_id, source_ref),

    CONSTRAINT allocation_rule_applicability_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(rule_version) <> ''
        )
);
