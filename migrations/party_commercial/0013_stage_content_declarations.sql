-- 阶段内容声明族（族 B 点读，ADR-0058）：收寄资格、终局规则、取消授权目录按拥有
-- 规则对象版本挂，不并进解析视图，也不合成一张表——ADR-0042 的归属纪律：声明按
-- 拥有对象挂，合成会把三条归属抹掉。
--
-- 产品与合同是采用方，不是本族的主键。收寄资格与终局规则归接单规则包（kind=4）；
-- 取消授权目录归授权规则（kind=9）。
--
-- 领域要求「至少一行」子声明。SQL 表达不了「子表至少一行」：无父行 = 未配置
-- （found=false）；父行在场而子行空/坏 = 装载 error，不得折成未配置。
--
-- 本表**不进 ViewRevision**（open-decisions D-4）：声明改动与选择无关。
--
-- 号段：B5 固定 0012；B7 规则包正文预留 0014。本批固定 0013。

-- 一行是一份已生效接单规则包的收寄资格声明壳。允许来源与资格引用分两张子表：
-- 资格引用允许显式空（真没有硬资格），允许来源不允许空。
CREATE TABLE party_commercial.intake_qualification_content (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT intake_qualification_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT intake_qualification_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    -- 4 = AcceptanceRulePackageObject。
    CONSTRAINT intake_qualification_content_rule_package_only
        CHECK (object_kind = 4)
);

CREATE TABLE party_commercial.intake_allowed_source (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    source_kind    text        NOT NULL,

    CONSTRAINT intake_allowed_source_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, source_kind),

    CONSTRAINT intake_allowed_source_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.intake_qualification_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT intake_allowed_source_closed_set
        CHECK (source_kind IN ('NODE_INTAKE', 'OFFSITE_PICKUP'))
);

CREATE TABLE party_commercial.intake_qualification_ref (
    tenant_id       text        NOT NULL,
    object_kind     smallint    NOT NULL,
    object_id       text        NOT NULL,
    version_label   text        NOT NULL,
    rule_reference  text        NOT NULL,

    CONSTRAINT intake_qualification_ref_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, rule_reference),

    CONSTRAINT intake_qualification_ref_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.intake_qualification_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT intake_qualification_ref_not_blank
        CHECK (btrim(rule_reference) <> '')
);

-- 一行是一份已生效接单规则包的终局规则声明壳。子表按责任结果封闭四值进主键。
CREATE TABLE party_commercial.final_rule_content (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT final_rule_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT final_rule_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    CONSTRAINT final_rule_content_rule_package_only
        CHECK (object_kind = 4)
);

CREATE TABLE party_commercial.final_rule_declaration (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    outcome        text        NOT NULL,
    final_kind     text        NOT NULL,

    CONSTRAINT final_rule_declaration_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, outcome),

    CONSTRAINT final_rule_declaration_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.final_rule_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT final_rule_declaration_closed_set
        CHECK (outcome IN (
            'EFFECTIVE_DELIVERY',
            'RETURN_COMPLETED',
            'SERVICE_TERMINATED',
            'REGULATORY_DISPOSITION'
        )),

    CONSTRAINT final_rule_declaration_kind_not_blank
        CHECK (btrim(final_kind) <> '')
);

-- 一行是一份已生效授权规则的取消授权目录壳。子表按请求方封闭二值进主键。
CREATE TABLE party_commercial.cancellation_authority_content (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT cancellation_authority_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT cancellation_authority_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    -- 9 = AuthorizationRuleObject。
    CONSTRAINT cancellation_authority_content_authorization_rule_only
        CHECK (object_kind = 9)
);

CREATE TABLE party_commercial.cancellation_authority_declaration (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    party          text        NOT NULL,
    rule_reference text        NOT NULL,

    CONSTRAINT cancellation_authority_declaration_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, party),

    CONSTRAINT cancellation_authority_declaration_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.cancellation_authority_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT cancellation_authority_declaration_closed_set
        CHECK (party IN ('CUSTOMER', 'OPERATIONS')),

    CONSTRAINT cancellation_authority_declaration_rule_not_blank
        CHECK (btrim(rule_reference) <> '')
);
