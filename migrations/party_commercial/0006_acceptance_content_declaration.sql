-- 接受内容声明（ADR-0042）。两张表，因为它们归**不同的拥有对象**：适用校验组与人工复核
-- 指令归接单规则包，待路由许可归服务产品。合成一张表会把这条归属抹掉，而 UC 正是按对象
-- 分别判给它们的。
--
-- 行属实例半边（真实校验组适用性、复核条件与许可依据待提供）；模式属机制半边。今天两表
-- 皆空，读口交回 found=false，消费方据此停在未决或不走待路由——两处的 false 语义相反，
-- 逐表在下面各自写明。

-- 一行是一份规则包正文的接受内容声明。
--
-- 组集合与人工复核指令同属一行：领域要求「空组集合等于无条件接受，规则包表达不了那种
-- 东西」且「人工复核不是默认步骤，未明确即未配置」，两者缺一都不成立一份完整声明，
-- 分两行会让一半在场的中间态存得下来。组集合放子表，见下。
--
-- manual_review 的 CHECK 镜像 domain.ManualReviewDirective 的**已声明**两值：零值
-- （未声明）不入表——一行在场就意味着声明过了，未声明由「查无此行」表达。
CREATE TABLE party_commercial.acceptance_rule_content (
    tenant_id     text        NOT NULL,
    object_kind   smallint    NOT NULL,
    object_id     text        NOT NULL,
    version_label text        NOT NULL,

    manual_review text        NOT NULL,
    declared_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT acceptance_rule_content_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT acceptance_rule_content_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(object_id) <> '' AND btrim(version_label) <> ''),

    -- 4 = AcceptanceRulePackageObject。
    CONSTRAINT acceptance_rule_content_rule_package_only
        CHECK (object_kind = 4),

    CONSTRAINT acceptance_rule_content_manual_review_closed
        CHECK (manual_review IN ('REQUIRED', 'NOT_REQUIRED'))
);

-- 适用校验组逐行，主键含组名把「同一组声明两次」结构性挡住。
--
-- 外键连回上表并级联删除：一组适用性脱离它所属的那份声明没有意义，留下来会变成一份
-- 无主的、读不出归属的适用性。
--
-- check_group 的 CHECK 镜像 domain.AcceptanceCheckGroupType 封闭集；该集合随消费方实现的
-- 校验组增长，扩展先改领域再改这一条。
CREATE TABLE party_commercial.acceptance_rule_check_group (
    tenant_id     text     NOT NULL,
    object_kind   smallint NOT NULL,
    object_id     text     NOT NULL,
    version_label text     NOT NULL,
    check_group   text     NOT NULL,

    CONSTRAINT acceptance_rule_check_group_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, check_group),

    CONSTRAINT acceptance_rule_check_group_content_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.acceptance_rule_content
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT acceptance_rule_check_group_closed
        CHECK (check_group IN (
            'CUSTOMER_RELATIONSHIP',
            'LEGAL_ENTITY_AND_CONTRACT',
            'PRODUCT_AND_SERVICE',
            'MEMBER_BASELINE',
            'REQUIRED_DOCUMENT',
            'PRE_ACCEPTANCE_FINANCIAL_CONTROL',
            'NETWORK_REACHABILITY'
        ))
);

-- 待路由许可归服务产品，不归规则包（UC-PS-001 把它判给产品）。
--
-- basis_ref NOT NULL 是本表的要害：领域规定「许可必须携带依据，没有依据的许可与一次默认
-- 放行分不开」。可空的依据列会让一行无依据的许可存得下来，那一行读回来就是一次默认放行。
--
-- 这里的 found=false 与上表语义相反：上表的 false 是「未配置」（消费方停在未决，不默认
-- 全组适用也不默认免复核），本表的 false 是「未许可」（零值语义，消费方照常推进、只是
-- 不走待路由）。两者共用一张表就没法各自说清，这是分表的第二个理由。
CREATE TABLE party_commercial.pending_routing_permission (
    tenant_id     text        NOT NULL,
    object_kind   smallint    NOT NULL,
    object_id     text        NOT NULL,
    version_label text        NOT NULL,

    basis_ref     text        NOT NULL,
    declared_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pending_routing_permission_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT pending_routing_permission_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 1 = ServiceProductObject。
    CONSTRAINT pending_routing_permission_service_product_only
        CHECK (object_kind = 1)
);
