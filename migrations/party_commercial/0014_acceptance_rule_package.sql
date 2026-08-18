-- 接单规则包正文（族 B 点读，open-decisions D-3：五维适用性照存但不参与选择）。
--
-- 父子两表。领域要求至少一条规则（空包等于无条件接受），因此零子行不是「显式空约定」，
-- 是坏数据：有父无子时装载走 error，不得折成未配置。无父行才是 found=false。
--
-- 本表**不进 ViewRevision**。ResolveCommercialBasis 选包只看版本壳的范围与选用区间，
-- 不看这五维；进了视图，一次与选择无关的正文改动会把该范围全部在途解析判成已失效。
-- 日后若改为参与选择，走新决策与新迁移，不改本表族归属的既成事实（ADR-0059）。
--
-- 号段 0013 留给阶段内容声明族（B6）；本批固定 0014。

-- 一行是一份已生效接单规则包版本的正文壳：选择信息的五个维度。
--
-- 适用范围的 scope / 期间与版本壳上的 scope_ref / 生效区间是两份事实。选包走壳，
-- 这里照存正文——存内容不等于参与选择。
CREATE TABLE party_commercial.acceptance_rule_package (
    tenant_id           text        NOT NULL,
    object_kind         smallint    NOT NULL,
    object_id           text        NOT NULL,
    version_label       text        NOT NULL,

    service_product_id  text        NOT NULL,
    contract_id         text        NOT NULL,
    legal_entity_ref    text        NOT NULL,
    scope_ref           text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    declared_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT acceptance_rule_package_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT acceptance_rule_package_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(service_product_id) <> ''
            AND btrim(contract_id) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),

    -- 4 = AcceptanceRulePackageObject。
    CONSTRAINT acceptance_rule_package_kind_only
        CHECK (object_kind = 4),

    CONSTRAINT acceptance_rule_package_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 按分类归档的规则引用。主键含分类与引用，挡住同一分类下同一引用登记两次。
--
-- category CHECK 镜像 domain.RuleCategory 五值封闭集。集外取值整行拒写。
-- AssembledRule 结构上只有分类与引用两列，表上同样没有阈值或取值列。
CREATE TABLE party_commercial.acceptance_rule_package_rule (
    tenant_id       text     NOT NULL,
    object_kind     smallint NOT NULL,
    object_id       text     NOT NULL,
    version_label   text     NOT NULL,
    rule_category   text     NOT NULL,
    rule_reference  text     NOT NULL,

    CONSTRAINT acceptance_rule_package_rule_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, rule_category, rule_reference),

    CONSTRAINT acceptance_rule_package_rule_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.acceptance_rule_package
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT acceptance_rule_package_rule_not_blank
        CHECK (btrim(rule_reference) <> ''),

    CONSTRAINT acceptance_rule_package_rule_category_closed
        CHECK (rule_category IN (
            'MINIMUM_INGRESS_IDENTITY',
            'SHIPMENT_INVARIANT',
            'PRODUCT_AND_CONTRACT_DOCUMENT',
            'REGULATORY_SOURCE_DOCUMENT',
            'CROSS_FIELD_CONDITION'
        ))
);
