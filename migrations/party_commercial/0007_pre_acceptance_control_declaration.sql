-- 客户合同版本对接受前财务控制的声明（`PAR-COM-15`，ADR-0042 的归属纪律）。
--
-- 一行是一条 domain.PreAcceptanceControlDeclaration：这份合同的这个范围**要不要**接受前
-- 财务控制，以及说`不适用`时凭什么。
--
-- 拥有对象是**客户合同版本**（object_kind=2）而不是接受前财务控制策略版本（第 5 类）：
-- `PAR-COM-15` 列在合同版本下，而且`不适用`的依据本身就是合同层面的话——策略版本回答
-- 「控制怎么做」，回答不了「这份合同要不要」。
--
-- 本表**不存控制方式，也不存实际采用的政策版本**：那属结算政策的解析（ADR-0044），已有
-- 自己的登记面。两层压成一张表就会让人从结算方式倒推控制要不要，而 `pn-02-w03` 明写
-- 账期不能推导无需信用校验。
--
-- not_applicable_basis 的可空性与 requirement 绑成一条 CHECK，这是本表的要害：CONTEXT
-- 要求「合同明确无接受前财务控制时必须保存商业不适用依据，不能用缺失结果或默认通过
-- 代替」。一行`不适用`而依据为空，读回来就是一次默认放行；反过来`要求控制`带着依据也
-- 读不出含义（那条依据说的是「凭什么不控制」），两头都拦。
--
-- requirement 的 CHECK 镜像 domain.PreAcceptanceControlRequirement 的**已声明**两值：
-- 零值（未声明）不入表——一行在场就意味着声明过了，未声明由「查无此行」表达。

CREATE TABLE party_commercial.pre_acceptance_control_declaration (
    tenant_id              text        NOT NULL,
    object_kind            smallint    NOT NULL,
    object_id              text        NOT NULL,
    version_label          text        NOT NULL,

    requirement            text        NOT NULL,
    not_applicable_basis   text,
    declared_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pre_acceptance_control_declaration_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT pre_acceptance_control_declaration_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(object_id) <> '' AND btrim(version_label) <> ''),

    -- 2 = CustomerContractObject。
    CONSTRAINT pre_acceptance_control_declaration_contract_only
        CHECK (object_kind = 2),

    CONSTRAINT pre_acceptance_control_declaration_requirement_closed
        CHECK (requirement IN ('REQUIRED', 'NOT_APPLICABLE')),

    -- 三值逻辑那一眼：可空列先 IS NULL / IS NOT NULL 再用，没有一条比较式能单独以 NULL
    -- 决定约束。
    CONSTRAINT pre_acceptance_control_declaration_basis_matches_requirement
        CHECK (
            (requirement = 'NOT_APPLICABLE'
                AND not_applicable_basis IS NOT NULL
                AND btrim(not_applicable_basis) <> '')
            OR
            (requirement = 'REQUIRED' AND not_applicable_basis IS NULL)
        )
);
