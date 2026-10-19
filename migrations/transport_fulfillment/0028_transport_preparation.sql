-- 运输准备结果（票 demo-chain-gaps/18，ADR-0201 决定六至八）。
--
-- 一段计划履约段对一个载运对象按一个规则版本只有一份当前有效结果：主键就是这四维（ADR-0201 决定七）。只插不改：
-- 同一身份再来时按依据摘要答已有结果或结果冲突，不覆盖原结果。计划段引用是 NR 的「版本#序位」，按 NR 拼写原样保存；
-- 改路出新版本就是新引用，与原结果并存。
--
-- 段的方向与窗口、需求量快照、候选与逐池依据都照判断当时写下：班次与池此后会变，这一份结果说的是当时按什么判的。
-- 候选与需求量是快照，各存一列 jsonb，不拆子表：它们只随这一行读回，不被别处按行引用或汇总。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL。
CREATE TABLE transport_fulfillment.transport_preparation (
    tenant_id         text        NOT NULL,
    planned_leg_ref   text        NOT NULL,
    object_ref        text        NOT NULL,
    rule_version      text        NOT NULL,

    connection_code   text        NOT NULL,
    from_node         text        NOT NULL,
    to_node           text        NOT NULL,
    window_start      timestamptz NOT NULL,
    window_end        timestamptz NOT NULL,
    demand_source     text        NOT NULL,
    demand_anchor     text,
    demand            jsonb       NOT NULL,
    outcome           text        NOT NULL,
    selected_schedule text,
    candidates        jsonb       NOT NULL,
    input_digest      text        NOT NULL,
    judged_at         timestamptz NOT NULL,
    recorded_at       timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT transport_preparation_pkey
        PRIMARY KEY (tenant_id, planned_leg_ref, object_ref, rule_version),

    CONSTRAINT transport_preparation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(planned_leg_ref) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(rule_version) <> ''
            AND btrim(connection_code) <> ''
            AND btrim(from_node) <> ''
            AND btrim(to_node) <> ''
            AND btrim(input_digest) <> ''
        ),

    CONSTRAINT transport_preparation_direction_distinct
        CHECK (from_node <> to_node),

    CONSTRAINT transport_preparation_window_ordered
        CHECK (window_end >= window_start),

    CONSTRAINT transport_preparation_demand_source_closed
        CHECK (demand_source IN ('declaration')),

    CONSTRAINT transport_preparation_demand_anchor_not_blank
        CHECK (demand_anchor IS NULL OR btrim(demand_anchor) <> ''),

    CONSTRAINT transport_preparation_outcome_closed
        CHECK (outcome IN ('NOT_CONFIGURED', 'NO_CURRENT_OPPORTUNITY', 'CAPACITY_UNCONFIRMED',
                           'OPPORTUNITY_UNAVAILABLE', 'OPPORTUNITY_PREPARED')),

    -- 只有已准备带选中的班次；未配置一个候选也没有——空册没有可评的东西。
    CONSTRAINT transport_preparation_selection_matches_outcome
        CHECK (
            (outcome = 'OPPORTUNITY_PREPARED' AND selected_schedule IS NOT NULL AND btrim(selected_schedule) <> '')
            OR (outcome <> 'OPPORTUNITY_PREPARED' AND selected_schedule IS NULL)
        ),

    CONSTRAINT transport_preparation_snapshots_are_arrays
        CHECK (jsonb_typeof(demand) = 'array' AND jsonb_typeof(candidates) = 'array'),

    CONSTRAINT transport_preparation_not_configured_has_no_candidates
        CHECK (outcome <> 'NOT_CONFIGURED' OR jsonb_array_length(candidates) = 0)
);

-- 查阅册按租户列、新判的在前。
CREATE INDEX transport_preparation_judged
    ON transport_fulfillment.transport_preparation (tenant_id, judged_at DESC);
