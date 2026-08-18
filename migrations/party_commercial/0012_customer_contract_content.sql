-- 客户合同正文（族 B 点读，ADR-0042 的归属纪律：声明按拥有对象挂，不并进解析视图）。
--
-- 父子两表，因为领域允许「显式声明空」：CustomerContract 的 bindings 可以为空，而
-- 「合同正文未登记」与「合同已登记、但没对任何费用范围作约定」是两件事。一张表的
-- 零行分不开这两者——无父行 = 未配置；有父行零子行 = 明确的空约定，FinancialControlFor
-- 对任何范围答「不存在」而不是「不适用」。
--
-- 本表**不进 ViewRevision**（open-decisions D-4）：绑定改动与选择无关，进了会把该范围
-- 全部在途解析判成已失效。与 0007（合同版本级「要不要」控制）并列不并表——语义不同。
--
-- 号段跳过 0009–0011，留给价格/结算政策册与区间更正册（B2/B3）；本批固定 0012。

-- 一行是一份已生效客户合同版本的正文壳：它引用哪个接单规则包。
--
-- rule_package_id 必存：NewCustomerContract 必须拿到规则包引用，而版本壳的
-- references[AcceptanceRulePackageObject] 允许缺席（declaredReferences 不强制），
-- 所以不能省掉这一列去从壳上取。装载时若壳上也指名了，必须与本列相等，不等整行拒装，
-- 不静默选一处（open-decisions F-3）。
CREATE TABLE party_commercial.customer_contract_content (
    tenant_id        text        NOT NULL,
    object_kind      smallint    NOT NULL,
    object_id        text        NOT NULL,
    version_label    text        NOT NULL,

    rule_package_id  text        NOT NULL,
    declared_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_contract_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT customer_contract_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(rule_package_id) <> ''
        ),

    -- 2 = CustomerContractObject。
    CONSTRAINT customer_contract_content_contract_only
        CHECK (object_kind = 2)
);

-- 合同按费用范围对财务控制的约定。同一范围两条结构性挡住（主键含 charge_scope_ref）。
--
-- 要害 CHECK：policy_id 与 inapplicability_basis 恰有一个非空。它镜像
-- FinancialControlBinding「要么适用一份指名的策略，要么显式不适用并记录依据，零值
-- 两者都不是」。两列都空的行读回来就是一次零值绑定，而零值绑定「读不成允许通过」
-- 只在领域里成立——库上放行一行两空，两个 New* 都构造不出它。
--
-- 可空列先 IS NULL / IS NOT NULL 再用，没有一条比较式能单独以 NULL 决定约束。
CREATE TABLE party_commercial.customer_contract_control_binding (
    tenant_id              text        NOT NULL,
    object_kind            smallint    NOT NULL,
    object_id              text        NOT NULL,
    version_label          text        NOT NULL,
    charge_scope_ref       text        NOT NULL,

    policy_id              text,
    inapplicability_basis  text,

    CONSTRAINT customer_contract_control_binding_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, charge_scope_ref),

    CONSTRAINT customer_contract_control_binding_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.customer_contract_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT customer_contract_control_binding_not_blank
        CHECK (btrim(charge_scope_ref) <> ''),

    CONSTRAINT customer_contract_control_binding_exactly_one
        CHECK (
            (policy_id IS NOT NULL
                AND btrim(policy_id) <> ''
                AND inapplicability_basis IS NULL)
            OR
            (policy_id IS NULL
                AND inapplicability_basis IS NOT NULL
                AND btrim(inapplicability_basis) <> '')
        )
);
