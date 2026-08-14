-- ETA 当前预测、可见性缺口与客户通知。
--
-- eta_prediction：键=租户+包裹+里程碑。库只管当前版——刷新换版是同一行改写，
-- 历史版本由 prior_version 指回承担，不另起一行（CONTEXT 硬句 103 的「不覆盖」
-- 由领域 Refresh 保原值对象，不是由本表多版本行保）。来源口径封闭二值。
--
-- visibility_gap：键含窗口规则版本——新窗口不覆盖原判断。缺口只证明预期观察
-- 届满未得，行内没有延误/遗失列。
--
-- customer_notification：按披露身份三维（客户+发作期+决定时间）定位，同一披露
-- 不重发。过程节点（已生成/提交渠道/失败等）进 jsonb 数组，分别记录不覆盖。
--
-- 租户是最高数据隔离边界（ADR-0003）：TrackedParcelReference 与客户账户都只是
-- 租户内引用，缺租户维两个租户的同名键会共用一行。带 NULL 列的 CHECK 一律走
-- IS NULL 显式分支。

CREATE TABLE visibility_exception.eta_prediction (
    tenant_id      text        NOT NULL,
    parcel_ref     text        NOT NULL,
    milestone_ref  text        NOT NULL,

    version_id     text        NOT NULL,
    source         text        NOT NULL,
    inputs_ref     text        NOT NULL,
    model_ref      text        NOT NULL,
    range_from     timestamptz NOT NULL,
    range_to       timestamptz NOT NULL,
    confidence_ref text        NOT NULL,
    predicted_at   timestamptz NOT NULL,
    prior_version  text,

    CONSTRAINT eta_prediction_pkey PRIMARY KEY (tenant_id, parcel_ref, milestone_ref),

    CONSTRAINT eta_prediction_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(milestone_ref) <> ''
            AND btrim(version_id) <> ''
            AND btrim(inputs_ref) <> ''
            AND btrim(model_ref) <> ''
            AND btrim(confidence_ref) <> ''
        ),
    CONSTRAINT eta_prediction_source_closed
        CHECK (source IN ('CARRIER_PROVIDED', 'OPERATOR_DERIVED')),
    CONSTRAINT eta_prediction_range_order
        CHECK (range_to > range_from),
    CONSTRAINT eta_prediction_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version_id)
        )
);

CREATE TABLE visibility_exception.visibility_gap (
    tenant_id        text        NOT NULL,
    parcel_ref       text        NOT NULL,
    expectation_ref  text        NOT NULL,
    window_rule_ref  text        NOT NULL,

    window_end       timestamptz NOT NULL,
    formed_at        timestamptz NOT NULL,

    CONSTRAINT visibility_gap_pkey PRIMARY KEY (
        tenant_id, parcel_ref, expectation_ref, window_rule_ref
    ),

    CONSTRAINT visibility_gap_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(expectation_ref) <> ''
            AND btrim(window_rule_ref) <> ''
        ),
    -- 窗口未届满构不成缺口（领域独立哨兵）；库面同样拦住提前写入。
    CONSTRAINT visibility_gap_window_elapsed
        CHECK (formed_at > window_end)
);

CREATE TABLE visibility_exception.customer_notification (
    tenant_id               text        NOT NULL,
    notification_id         text        NOT NULL,

    customer_ref            text        NOT NULL,
    episode_id              text        NOT NULL,
    decided_at              timestamptz NOT NULL,

    disclosure_policy_ref   text        NOT NULL,
    disclosure_conclusion   text        NOT NULL,
    content_ref             text        NOT NULL,
    deadline                timestamptz NOT NULL,
    channel_ref             text        NOT NULL,
    obligation_ref          text        NOT NULL,
    milestones              jsonb       NOT NULL,

    CONSTRAINT customer_notification_pkey PRIMARY KEY (tenant_id, notification_id),

    CONSTRAINT customer_notification_disclosure_identity
        UNIQUE (tenant_id, customer_ref, episode_id, decided_at),

    CONSTRAINT customer_notification_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(notification_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(episode_id) <> ''
            AND btrim(disclosure_policy_ref) <> ''
            AND btrim(content_ref) <> ''
            AND btrim(channel_ref) <> ''
            AND btrim(obligation_ref) <> ''
        ),
    -- 通知只由披露格生成；非披露没有可通知的内容。
    CONSTRAINT customer_notification_disclose_only
        CHECK (disclosure_conclusion IS NOT DISTINCT FROM 'DISCLOSE'),
    -- 后续节点的封闭六值由领域 RecordMilestone 重建口拦；PostgreSQL 不允许
    -- CHECK 含子查询，不能在库内对 jsonb 数组逐元 IN。
    CONSTRAINT customer_notification_milestones_shape
        CHECK (
            jsonb_typeof(milestones) = 'array'
            AND jsonb_array_length(milestones) >= 1
            AND milestones->0->>'milestone' IS NOT DISTINCT FROM 'GENERATED'
            AND milestones->0->>'recordedAt' IS NOT NULL
            AND btrim(milestones->0->>'recordedAt') <> ''
        )
);
