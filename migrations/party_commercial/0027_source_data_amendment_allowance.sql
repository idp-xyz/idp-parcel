-- 资料修订允许声明（票 party-commercial-context-gaps/10，ADR-0120）：接单规则包版本下按
--（资料组 × 资料修订阶段 × 修订意图）逐格登记「能不能改」的矩阵，是 0013 两族之后的第三族
-- 阶段内容声明。PS 的 ports.SourceDataRuleDeclaration 头注禁它自带矩阵，此前仓内只有替身——
-- 语言在（UC-PS-002 / AT-PS-020 / PS CONTEXT「显式清空必须由版本化字段和阶段规则允许」）、
-- 格没有，租户即便有了规则也无处登。
--
-- 形照 0013：父行是声明壳，子行是格；产品与合同是采用方，不是本族的主键（ADR-0058 决定一的
-- 同一条纪律）。不用一个 JSON 列：逐格可查、逐格可约束（ADR-0115 Decision 二同一判据）——三个
-- 封闭集由 CHECK 守住，JSON 守不住；消费方查的是一格，不该为一格解一整份。
--
-- 父行的 closed 是登记方对「缺格怎么读」的显式选择（MCP-1 代裁 Q3）：false = 缺格读「未声明」
--（请求转复核），true = 缺格读「不允许」。它是一格布尔而不是产品默认，因为「等复核」还是「被拒」
-- 是客户看得见的差别，该由登记方说。子行只登 ALLOWED / DISALLOWED，永不登「未声明」——「未声明」
-- 是缺格的读法，不是一行的取值；登成一行会让「没说」与「说了『我不说』」在表里分不开。
--
-- 领域要求「未封闭至少一格」；SQL 表达不了「子表至少一行」。与 0013 同一句：无父行 = 未配置
--（found=false）；父行在场而正文立不住 = 装载 error，不得折成未配置。closed=true 零行是一句显式的
-- 话（这一版什么都不许改），合法登记。
--
-- 阶段六格与意图三格的原词属 parcel-shipment（domain.AmendmentStage / AmendmentIntent，ADR-0118），
-- 这里只镜像不另定、一格不拆不加（ADR-0120 Decision 五）；PS 加格，这里同步一次迁移放宽 CHECK。
-- 资料组引用是开放引用：词表属 PAR-COM-13 的实例登记，只查非空。
--
-- 本表不进 ViewRevision（open-decisions D-4）：声明改动与选择无关。行属实例半边：今天没有租户，
-- 两表皆空；表上没有任何一格默认允许。

CREATE TABLE party_commercial.source_data_amendment_content (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    closed         boolean     NOT NULL,
    declared_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT source_data_amendment_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT source_data_amendment_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
        ),

    -- 4 = AcceptanceRulePackageObject。
    CONSTRAINT source_data_amendment_content_rule_package_only
        CHECK (object_kind = 4)
);

-- 一行是一格：某一资料组在某一阶段以某一意图，允许或不允许。主键含三维，同一格不两行。
CREATE TABLE party_commercial.source_data_amendment_allowance (
    tenant_id       text     NOT NULL,
    object_kind     smallint NOT NULL,
    object_id       text     NOT NULL,
    version_label   text     NOT NULL,
    data_group_ref  text     NOT NULL,
    stage           text     NOT NULL,
    intent          text     NOT NULL,
    allowance       text     NOT NULL,

    CONSTRAINT source_data_amendment_allowance_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent),

    CONSTRAINT source_data_amendment_allowance_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.source_data_amendment_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT source_data_amendment_allowance_group_not_blank
        CHECK (btrim(data_group_ref) <> ''),

    -- 镜像 parcel-shipment domain.AmendmentStage 六格原词（UC-PS-002 阶段表）。
    CONSTRAINT source_data_amendment_allowance_stage_closed_set
        CHECK (stage IN (
            'ACCEPTED_NOT_YET_RECEIVED',
            'RECEIVED_OR_MEASURED',
            'LABELLED_OR_BAGGED',
            'CUSTOMS_DATA_FORMING_NOT_SUBMITTED',
            'CUSTOMS_SUBMITTED',
            'CASE_CLOSED_OR_SERVICE_COMPLETED'
        )),

    -- 镜像 parcel-shipment domain.AmendmentIntent 三格原词。
    CONSTRAINT source_data_amendment_allowance_intent_closed_set
        CHECK (intent IN ('SUPPLEMENT', 'CORRECTION', 'EXPLICIT_CLEAR')),

    -- 两值，不收 NOT_DECLARED：缺格的读法由父行 closed 决定，不是一行的取值。
    CONSTRAINT source_data_amendment_allowance_two_valued
        CHECK (allowance IN ('ALLOWED', 'DISALLOWED'))
);
