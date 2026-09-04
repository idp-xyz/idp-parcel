-- 异常披露规则目录（`PAR-VIS-07` 的「披露和自动发布范围」半边；渠道半边在 0010）与
-- 披露决定登记册。在此之前 DecideDisclosure 没有生产调用方：`UC-VE-006` 的编排从「决定
-- 已经有了」开始，而没有任何东西形成它、也没有地方登记它。
--
-- 内容属实例半边（合同条款、披露策略、受限监管内容、人工/自动授权范围待登记），两张
-- 目录表在首发是空的。空表时读口交回「未配置」，编排停在未决——没有规则时既不能说
-- 「披露」也不能说「不披露」，后者同样是一次没人作过的披露决定。
--
-- 目录拆成「版本」与「条目」两张表，形状同 0012 披露策略：版本引用同时就是决定与通知
-- 策略共用的那个披露策略引用——决定带着它走，notification_policy 按它取渠道与时限，两张
-- 目录靠这一个值接上。适用版本按决定时刻取当前未闭区间那一版（同分诊规则：判的是手上
-- 这个活信号）；一租户至多一个未闭区间由部分唯一索引守。
CREATE TABLE visibility_exception.exception_disclosure_rule_version (
    tenant_id      text        NOT NULL,
    rule_version   text        NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    approved_by    text        NOT NULL,

    CONSTRAINT exception_disclosure_rule_version_pkey PRIMARY KEY (tenant_id, rule_version),

    CONSTRAINT exception_disclosure_rule_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(approved_by) <> ''
        ),
    CONSTRAINT exception_disclosure_rule_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX exception_disclosure_rule_version_one_open_per_tenant
    ON visibility_exception.exception_disclosure_rule_version (tenant_id)
    WHERE effective_to IS NULL;

-- 条目按（客户账户 + 信号类型 + 可信度）建键：「客户可见性必须根据服务产品、客户合同、
-- 信号可信度、影响范围、预计客户影响和信息披露规则形成版本化决定」（CONTEXT）——合同
-- 落在客户账户上，可信度与分诊条目同维；影响范围与预计客户影响今天没有可查的登记维，
-- 等它们有形状再扩键，不先替租户拟一格。
--
-- 三个布尔/内容列对应 `UC-VE-006` 的两道门：disclosable 是「披露条件成不成立」，
-- auto_release 是「批准范围允不允许自动发布」（`PAR-VIS-07`：默认草稿经授权确认，严格
-- 范围才可自动发布）。两条 shape 约束把矛盾输入挡在库外：披露必带内容快照引用、不披露
-- 必不带（与 DecideDisclosure 的构造门同一条线）；不披露就谈不上自动发布。
CREATE TABLE visibility_exception.exception_disclosure_rule_entry (
    tenant_id            text    NOT NULL,
    rule_version         text    NOT NULL,
    customer_account_ref text    NOT NULL,
    signal_kind          text    NOT NULL,
    confidence_ref       text    NOT NULL,

    disclosable          boolean NOT NULL,
    auto_release         boolean NOT NULL,
    content_ref          text,

    CONSTRAINT exception_disclosure_rule_entry_pkey
        PRIMARY KEY (tenant_id, rule_version, customer_account_ref, signal_kind, confidence_ref),

    CONSTRAINT exception_disclosure_rule_entry_version_fkey
        FOREIGN KEY (tenant_id, rule_version)
        REFERENCES visibility_exception.exception_disclosure_rule_version (tenant_id, rule_version)
        ON DELETE RESTRICT,

    CONSTRAINT exception_disclosure_rule_entry_not_blank
        CHECK (
            btrim(customer_account_ref) <> ''
            AND btrim(signal_kind) <> ''
            AND btrim(confidence_ref) <> ''
        ),

    CONSTRAINT exception_disclosure_rule_entry_content_pairs_with_disclosable
        CHECK (
            (disclosable AND content_ref IS NOT NULL AND btrim(content_ref) <> '')
            OR (NOT disclosable AND content_ref IS NULL)
        ),

    CONSTRAINT exception_disclosure_rule_entry_auto_release_requires_disclosable
        CHECK (disclosable OR NOT auto_release)
);

-- 披露决定登记册：三态结论全部入册——「披露条件不成立或授权不足时分别形成暂不披露
-- 或待授权结果」（CONTEXT），暂不披露与待授权是已作出的决定，不是没有决定；不记它们，
-- 同一发作期就会被反复重判。
--
-- 行身份取决定的三维（客户 + 发作期 + 决定时刻），与 0006 里客户通知定位披露决定的
-- 三维同一口径；同（发作期+客户）下决定时刻最晚的那一份是当前决定，读口按此取。版本
-- 只增不改写：待授权转披露由授权那一步另起一行新决定，不改旧行。
--
-- 结论取封闭三值（domain.DisclosureConclusion）；披露必带内容快照引用、其余两态必不带
-- ——与 DecideDisclosure 的构造门同一条线，库上的约束是第二道网。
CREATE TABLE visibility_exception.disclosure_decision (
    tenant_id             text        NOT NULL,
    episode_id            text        NOT NULL,
    customer_ref          text        NOT NULL,
    decided_at            timestamptz NOT NULL,

    disclosure_policy_ref text        NOT NULL,
    conclusion            text        NOT NULL,
    content_ref           text,

    CONSTRAINT disclosure_decision_pkey
        PRIMARY KEY (tenant_id, episode_id, customer_ref, decided_at),

    CONSTRAINT disclosure_decision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(episode_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(disclosure_policy_ref) <> ''
        ),

    CONSTRAINT disclosure_decision_conclusion_closed_set
        CHECK (conclusion IN ('DISCLOSE', 'NOT_YET_DISCLOSABLE', 'AWAITING_AUTHORIZATION')),

    CONSTRAINT disclosure_decision_content_pairs_with_disclose
        CHECK (
            (conclusion = 'DISCLOSE' AND content_ref IS NOT NULL AND btrim(content_ref) <> '')
            OR (conclusion <> 'DISCLOSE' AND content_ref IS NULL)
        )
);
