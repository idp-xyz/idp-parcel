-- 接单规则包的时点锚声明（ADR-0042：接受内容声明按对象归属建模）。
--
-- 一行是一条 domain.AsOfPolicy：某个已生效接单规则包为某一类下游判断声明「适用哪种时点
-- 语义、依据该政策的哪个版本」。它**不存时点值本身**——取值由消费方逐项形成，正是这一点
-- 让一个全局时间无法替代所有下游判断的时点。
--
-- 归属键取规则包版本的完整身份（租户+类别+对象+版本号），四维缺一不可：commercial_version
-- 的主键含 object_kind，所以对象标识只在同类别内唯一，少这一维会让另一类对象的同名版本
-- 串进来。类别固定为接单规则包（domain.AcceptanceRulePackageObject），CHECK 钉住它。
--
-- 主键含 judgment_type，把 domain.DeclareAsOfPolicies 的「同一判断不得两条」结构性落库：
-- 同一判断两个时点锚互相矛盾，取哪个都是掷硬币，那种形状在库内就不该存在。
--
-- judgment_type 的 CHECK 是 domain.JudgmentType 封闭集的镜像；该集合随「产生那类判断的
-- 规则」一并增长，扩展必须先改领域再改这一条。
--
-- 行属实例半边：真实语义与政策版本登记为 `PAR-COM-14`，今天没有租户因而本表为空，读口
-- 交回空声明、编排据以停在`未配置`——那是首发要停下的地方，不是要绕过的地方。

CREATE TABLE party_commercial.as_of_policy_declaration (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,

    judgment_type  text        NOT NULL,
    semantics_ref  text        NOT NULL,
    policy_version text        NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT as_of_policy_declaration_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, judgment_type),

    CONSTRAINT as_of_policy_declaration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(semantics_ref) <> ''
            AND btrim(policy_version) <> ''
        ),

    -- 4 = AcceptanceRulePackageObject。声明只挂接单规则包：服务产品那半边的声明
    -- （待路由许可）按 ADR-0042 归服务产品，另表承载。
    CONSTRAINT as_of_policy_declaration_rule_package_only
        CHECK (object_kind = 4),

    -- 镜像 domain.JudgmentType。新增判断类型先改领域封闭集，再改这一条。
    CONSTRAINT as_of_policy_declaration_judgment_closed
        CHECK (judgment_type IN ('NETWORK_REACHABILITY', 'PRE_ACCEPTANCE_FINANCIAL_CONTROL'))
);
