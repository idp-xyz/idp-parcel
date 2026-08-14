-- 版本化规则目录：里程碑映射（`PAR-VIS-01`）与分诊规则（`PAR-VIS-05`）。
--
-- 两份目录的**内容**属实例半边：没有租户就没有已发布的映射版本与分诊阈值，两张
-- 目录因此在首发是空的。空目录不是缺陷，是如实的空白——视图据此交回「未配置」，
-- 编排照 CONTEXT 走未归类与人工复核，不虚构里程碑，也不自动建案。
--
-- 每份目录拆成「版本」与「条目」两张表，是因为消费方必须分得开两件事：目录整个
-- 没配（未配置）与目录配了但这一条没命中（已按 vN 判过，无可靠映射／无规则命中）。
-- 压成一张表，两者都表现为「查不到行」，而 CONTEXT 对它们的要求相反——前者等租户
-- 登记，后者是已经作出的判断，必须带着所依据的版本号进投影和分诊结论。

-- 里程碑映射版本。「标准追踪里程碑映射必须具有版本和适用范围」（CONTEXT 硬句）；
-- 「每个映射以不可覆盖版本表达……有效区间和发布批准责任」（`PAR-VIS-01`）。
CREATE TABLE visibility_exception.milestone_mapping_version (
    tenant_id       text        NOT NULL,
    mapping_version text        NOT NULL,

    -- 适用按事实的**业务发生时间**判定，不按系统处理时间：「新版本默认只作用于
    -- 生效后的事件」，而一条迟到三天才到达的事实，属于它发生那天的映射版本。
    effective_from  timestamptz NOT NULL,
    effective_to    timestamptz,
    approved_by     text        NOT NULL,

    CONSTRAINT milestone_mapping_version_pkey PRIMARY KEY (tenant_id, mapping_version),

    CONSTRAINT milestone_mapping_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(mapping_version) <> ''
            AND btrim(approved_by) <> ''
        ),
    CONSTRAINT milestone_mapping_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

-- 同一租户至多一个仍然有效（未闭区间）的映射版本。两个并存版本会让「选择当前适用
-- 的映射版本」有两个答案，而 CONTEXT 不许按最后到达或来源排名挑一个——库先挡住，
-- 适配器再对已闭区间的重叠兜一道错。
CREATE UNIQUE INDEX milestone_mapping_version_one_open_per_tenant
    ON visibility_exception.milestone_mapping_version (tenant_id)
    WHERE effective_to IS NULL;

-- 映射条目：某源上下文的某份已接受事实引用归到哪个标准里程碑。
--
-- 键取（源上下文 + 源事实引用），因为这是 `MilestoneMappingView.ClassifyFact` 唯一
-- 拿得到的可映射输入——`AcceptedSourceFact` 上没有源事实类型或来源代码字段。
-- `PAR-VIS-01` 要的是「源事实类型/代码 → 标准里程碑」，按类型建目录需要领域先给出
-- 那个字段；在它到位之前，本表按端口实际交出的那一维建键，不替领域发明第二套口径。
CREATE TABLE visibility_exception.milestone_mapping_entry (
    tenant_id       text NOT NULL,
    mapping_version text NOT NULL,
    source_context  text NOT NULL,
    source_fact_ref text NOT NULL,
    milestone_ref   text NOT NULL,

    CONSTRAINT milestone_mapping_entry_pkey
        PRIMARY KEY (tenant_id, mapping_version, source_context, source_fact_ref),

    CONSTRAINT milestone_mapping_entry_version_fkey
        FOREIGN KEY (tenant_id, mapping_version)
        REFERENCES visibility_exception.milestone_mapping_version (tenant_id, mapping_version)
        ON DELETE RESTRICT,

    -- 源上下文取投影可消费的封闭五处（domain.SourceContext）。库里多一格，就等于
    -- 承认某个没被业务所有者接受过的来源也能进投影。
    CONSTRAINT milestone_mapping_entry_source_set
        CHECK (source_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE'
        )),

    -- 条目必须给出里程碑。「没有可靠映射」由**没有这一行**表达，不由一行空里程碑
    -- 表达——后者会让「未归类」看起来像一次已作出的映射判断。
    CONSTRAINT milestone_mapping_entry_not_blank
        CHECK (btrim(source_fact_ref) <> '' AND btrim(milestone_ref) <> '')
);

-- 分诊规则版本。「防抖、宽限和恢复边界必须具有规则版本」「高可信、高影响且命中
-- 版本化分诊规则的信号可以自动建立或关联案件」（CONTEXT 硬句）。
CREATE TABLE visibility_exception.triage_rule_version (
    tenant_id      text        NOT NULL,
    rule_version   text        NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    approved_by    text        NOT NULL,

    CONSTRAINT triage_rule_version_pkey PRIMARY KEY (tenant_id, rule_version),

    CONSTRAINT triage_rule_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(approved_by) <> ''
        ),
    CONSTRAINT triage_rule_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX triage_rule_version_one_open_per_tenant
    ON visibility_exception.triage_rule_version (tenant_id)
    WHERE effective_to IS NULL;

-- 分诊条目：某信号类型在某可信度依据下走哪一格。
--
-- 键含可信度，因为四走向的分界正立在它上面：「高可信、高影响且命中版本化分诊规则
-- 的信号可以自动建立或关联案件；低可信、资料不足、可能重复或关联不明确的信号必须
-- 先进入分诊」。按类型单键建目录，等于宣布同一类型的信号不分可信度一律同一走向。
CREATE TABLE visibility_exception.triage_rule_entry (
    tenant_id      text NOT NULL,
    rule_version   text NOT NULL,
    signal_kind    text NOT NULL,
    confidence_ref text NOT NULL,
    outcome        text NOT NULL,

    CONSTRAINT triage_rule_entry_pkey
        PRIMARY KEY (tenant_id, rule_version, signal_kind, confidence_ref),

    CONSTRAINT triage_rule_entry_version_fkey
        FOREIGN KEY (tenant_id, rule_version)
        REFERENCES visibility_exception.triage_rule_version (tenant_id, rule_version)
        ON DELETE RESTRICT,

    -- 封闭四走向（domain.TriageOutcome）。库里多一格就是给分诊开了第五种结论。
    CONSTRAINT triage_rule_entry_outcome_set
        CHECK (outcome IN (
            'ATTACH_TO_EXISTING', 'AUTO_ESTABLISH', 'MANUAL_REVIEW', 'NO_CASE'
        )),

    CONSTRAINT triage_rule_entry_not_blank
        CHECK (btrim(signal_kind) <> '' AND btrim(confidence_ref) <> '')
);
