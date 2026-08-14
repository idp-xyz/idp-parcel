-- 索赔资格声明（`PAR-VIS-08` 的一角）：某合同责任范围承担哪些索赔类型。
--
-- 本目录**只承担资格审核七维里的一维**。CONTEXT 要求按「申请人授权、客户账户、合同
-- 版本、索赔时限、目标范围、重复关系和最低材料要求」判断资格，而 `EligibilityQuery`
-- 交出的只有客户账户、合同范围、目标范围与索赔类型——它连一个时间戳都没有，因此
-- 索赔时限算不了；申请人不在查询里，授权核不了；材料在证据聚合里、重复关系在
-- ClaimStore 里，都不是目录行。缺口已在 .scratch/ve-claim-eligibility-dimensions/ 立票。
--
-- 因此这张表只回答一件事，而这件事它回答得完整：**这个合同范围承不承担这个索赔
-- 类型**。承担与否不随材料补充而改变（改变的只能是合同，而换了合同范围就是另一个
-- 索赔项），所以由它得出的`不通过`是一次可以永久成立的判定——这一点要紧，因为
-- `ScreenEligibility` 是一次性的，审过就不再审，答错了没有第二次机会。
--
-- 拆成「声明」与「覆盖类型」两张表，照 party-commercial 的 `IntakeQualificationContent`
-- 那个形状：**只有声明在场，「不在集合内」才说得通**。没有声明行时，缺一个类型是
-- 「没人声明过」，不是「声明说不保」——把两者混同就会凭一张空表永久拒掉索赔。

CREATE TABLE visibility_exception.claim_contract_scope (
    tenant_id          text NOT NULL,
    contract_scope_ref text NOT NULL,

    -- 声明所属的规则版本。答复里没有版本字段，所以它随依据串进 basis——那串是
    -- 唯一会被 ScreenEligibility 永久记进索赔项的东西，而一次永久的`不通过`必须
    -- 追得回它依据的是哪一版声明。
    rule_version       text NOT NULL,
    approved_by        text NOT NULL,

    CONSTRAINT claim_contract_scope_pkey PRIMARY KEY (tenant_id, contract_scope_ref),

    CONSTRAINT claim_contract_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(contract_scope_ref) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(approved_by) <> ''
        )
);

CREATE TABLE visibility_exception.claim_covered_kind (
    tenant_id          text NOT NULL,
    contract_scope_ref text NOT NULL,
    claim_kind_ref     text NOT NULL,

    CONSTRAINT claim_covered_kind_pkey
        PRIMARY KEY (tenant_id, contract_scope_ref, claim_kind_ref),

    -- 覆盖行必须挂在已登记的声明下。孤立的覆盖行会让「声明在场」这个前提失真，
    -- 而上面那条永久判定完全建立在它之上。
    CONSTRAINT claim_covered_kind_scope_fkey
        FOREIGN KEY (tenant_id, contract_scope_ref)
        REFERENCES visibility_exception.claim_contract_scope (tenant_id, contract_scope_ref)
        ON DELETE RESTRICT,

    CONSTRAINT claim_covered_kind_not_blank
        CHECK (btrim(claim_kind_ref) <> '')
);
