-- ETA 客户可见性判断（UC-VE-003 结果语义，`AT-VE-187`、`AT-VE-170`、`AT-VE-052`）与它的租户规则册。
--
-- 判断针对一个货主客户账户、一件包裹，对一份候选内部 ETA 作出：引用候选 ETA 版本，保存质量评价（理由）、
-- 对客质量门槛版本（`PAR-VIS-03`）、合同与客户服务规则版本（`PAR-COM-07`）、结论、判断时点、判断版本与
-- 替代关系；结论为客户可见 ETA 时另存获准对客表达与停止展示边界。结论只有三格，不可见与待判断两格不带
-- 对客表达。一个账户在一件包裹上同时只有一条判断线，里程碑是判断的属性不是键的一维：客户视图只有一维 ETA，
-- 规则指名哪个里程碑的 ETA 是客户要的那一个。
--
-- 形状照 0028 的客户视图版本表：版本行一经写入不再改动，替代追加新版本行并回指前一版，原判断不覆盖
-- （UC-VE-003 规则「新 ETA、质量门槛、合同或客户服务规则版本生效时必须形成新的 ETA 客户可见性判断和替代
-- 关系」）；当前版由 eta_visibility_judgement_current 的显式标记指名，标记是唯一的可变处。
--
-- 候选 ETA 版本与来源口径存成裸文本、不设外键：eta_prediction（0006）只管当前行，被替代的预测版本读不回，
-- 而判断要留的正是「当时依据的是哪一版」。
--
-- 租户是最高数据隔离边界（ADR-0003）：客户账户、包裹引用都只是租户内引用。

CREATE TABLE visibility_exception.eta_visibility_judgement_version (
    tenant_id             text        NOT NULL,
    customer_account_id   text        NOT NULL,
    parcel_id             text        NOT NULL,
    version               text        NOT NULL,

    milestone_ref         text        NOT NULL,
    candidate_eta_version text        NOT NULL,
    candidate_source      text        NOT NULL,

    conclusion            text        NOT NULL,
    reason                text        NOT NULL,
    gate_version          text,
    service_rule_version  text,
    expression            text,
    stop_boundary_ref     text,

    judged_at             timestamptz NOT NULL,
    prior_version         text,
    recorded_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT eta_visibility_judgement_version_pkey
        PRIMARY KEY (tenant_id, customer_account_id, parcel_id, version),

    CONSTRAINT eta_visibility_judgement_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(version) <> ''
            AND btrim(milestone_ref) <> ''
            AND btrim(candidate_eta_version) <> ''
        ),
    CONSTRAINT eta_visibility_judgement_version_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version)
        ),
    CONSTRAINT eta_visibility_judgement_version_source_closed
        CHECK (candidate_source IN ('CARRIER_PROVIDED', 'OPERATOR_DERIVED')),

    -- 结论与字段成对：可见必带规则版本、表达与停止边界；不可见带规则版本、不带表达；待判断什么规则字段都不带
    -- ——没有适用规则就没有可借用的版本。库先挡一道，领域重建口再过一遍同样的不变量。带 NULL 列的比较一律先走
    -- IS NOT NULL / IS NULL 显式分支：NULL 参与比较得 UNKNOWN，CHECK 把 UNKNOWN 当通过。
    CONSTRAINT eta_visibility_judgement_version_shape
        CHECK (
            (conclusion = 'CUSTOMER_VISIBLE_ETA'
                AND reason = 'QUALITY_MET_GATE'
                AND gate_version IS NOT NULL AND btrim(gate_version) <> ''
                AND service_rule_version IS NOT NULL AND btrim(service_rule_version) <> ''
                AND expression IS NOT NULL AND expression IN ('POINT_VALUE', 'TIME_RANGE')
                AND stop_boundary_ref IS NOT NULL AND btrim(stop_boundary_ref) <> '')
            OR (conclusion = 'ETA_NOT_VISIBLE'
                AND reason = 'QUALITY_BELOW_GATE'
                AND gate_version IS NOT NULL AND btrim(gate_version) <> ''
                AND service_rule_version IS NOT NULL AND btrim(service_rule_version) <> ''
                AND expression IS NULL
                AND stop_boundary_ref IS NULL)
            OR (conclusion = 'ETA_VISIBILITY_PENDING'
                AND reason IN ('RULE_NOT_CONFIGURED', 'RULE_CONFLICT')
                AND gate_version IS NULL
                AND service_rule_version IS NULL
                AND expression IS NULL
                AND stop_boundary_ref IS NULL)
        )
);

CREATE TABLE visibility_exception.eta_visibility_judgement_current (
    tenant_id             text NOT NULL,
    customer_account_id   text NOT NULL,
    parcel_id             text NOT NULL,
    version               text NOT NULL,

    CONSTRAINT eta_visibility_judgement_current_pkey
        PRIMARY KEY (tenant_id, customer_account_id, parcel_id),

    -- 标记只能指一条存下来的版本行，新链条的前身因此都读得回。
    CONSTRAINT eta_visibility_judgement_current_names_stored_version
        FOREIGN KEY (tenant_id, customer_account_id, parcel_id, version)
        REFERENCES visibility_exception.eta_visibility_judgement_version
            (tenant_id, customer_account_id, parcel_id, version)
);

-- ETA 对客规则册（`PAR-VIS-03` 的对客质量门槛与 `PAR-COM-07` 的合同与客户服务规则；两者都是租户取值）。
--
-- 内容属实例半边（质量门槛、合同与客户服务规则待登记），表在首发是空的。空表或这个账户没有适用行时读口交回
-- 「未配置」，判断落「ETA 可见性待判断」——那是如实的空白，不是「不可见」，更不是按默认值判为可见。
--
-- 一行是一个账户的一版规则，适用按判断时点（effective_from 含、effective_to 不含）。可信度依据在内部 ETA
-- 上是不透明引用、没有可比大小的数值，所以门槛只能表达为「哪些依据达标」的集合；集合不能为空（空集合意味着
-- 永远不可见，那是另一个决定）。同一账户同一时点适用两行是规则冲突，读口交回「冲突」，判断落待判断，
-- 不任选其一；未闭区间至多一行由唯一索引先挡，闭区间重叠由读口兜。
--
-- 集合里的每一个依据都要非空白：读口把行重建成领域规则时，空白依据被领域层拒收，会被当作规则读不动、
-- 判断落未决，消费者永远回滚重投。坏行只能挡在写入口，所以库对元素也设一道。PostgreSQL 的 CHECK 不允许
-- 子查询，逐元检查只能包进 IMMUTABLE 函数。
CREATE FUNCTION visibility_exception.eta_qualifying_confidences_are_present(refs text[])
RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT NOT EXISTS (
        SELECT 1
          FROM unnest(refs) AS confidence(ref)
         WHERE confidence.ref IS NULL OR confidence.ref ~ '^\s*$'
    );
$$;

CREATE TABLE visibility_exception.eta_visibility_rule (
    tenant_id                text        NOT NULL,
    customer_account_ref     text        NOT NULL,
    service_rule_version     text        NOT NULL,

    gate_version             text        NOT NULL,
    target_milestone_ref     text        NOT NULL,
    qualifying_confidences   text[]      NOT NULL,
    expression               text        NOT NULL,
    stop_boundary_ref        text        NOT NULL,

    effective_from           timestamptz NOT NULL,
    effective_to             timestamptz,
    approved_by              text        NOT NULL,
    registered_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT eta_visibility_rule_pkey
        PRIMARY KEY (tenant_id, customer_account_ref, service_rule_version),

    CONSTRAINT eta_visibility_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_ref) <> ''
            AND btrim(service_rule_version) <> ''
            AND btrim(gate_version) <> ''
            AND btrim(target_milestone_ref) <> ''
            AND btrim(stop_boundary_ref) <> ''
            AND btrim(approved_by) <> ''
        ),
    CONSTRAINT eta_visibility_rule_qualifying_present
        CHECK (
            cardinality(qualifying_confidences) >= 1
            AND visibility_exception.eta_qualifying_confidences_are_present(qualifying_confidences)
        ),
    CONSTRAINT eta_visibility_rule_expression_closed
        CHECK (expression IN ('POINT_VALUE', 'TIME_RANGE')),
    CONSTRAINT eta_visibility_rule_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX eta_visibility_rule_one_open_per_customer
    ON visibility_exception.eta_visibility_rule (tenant_id, customer_account_ref)
    WHERE effective_to IS NULL;
