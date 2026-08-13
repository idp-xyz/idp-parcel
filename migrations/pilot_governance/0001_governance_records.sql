-- 治理记录三表：候选版本组、阶段评审决定、生产权威区间。治理是产品级机制，没有
-- 租户维——这里登记的是「本产品此刻拿什么去评审、谁在写生产」，全部是脱敏引用，
-- 真实客户、线路与阈值留在参数登记册。

-- 候选版本组：三样引用一次进入，不可扩张——表上没有任何 UPDATE 路径可依赖的列
-- 语义，适配器也不提供改写方法；范围扩大或版本变更生成新的组。
CREATE TABLE pilot_governance.candidate_version_set (
    set_id       text        NOT NULL,
    scope        text        NOT NULL,
    parameters   text        NOT NULL,
    rules        text        NOT NULL,
    formed_at    timestamptz NOT NULL,
    inserted_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT candidate_version_set_pkey PRIMARY KEY (set_id),
    CONSTRAINT candidate_version_set_not_blank
        CHECK (
            btrim(set_id) <> ''
            AND btrim(scope) <> ''
            AND btrim(parameters) <> ''
            AND btrim(rules) <> ''
        )
);

-- 阶段评审决定：幂等键=（评审目标+候选版本组），同键第二份由主键拦住译成`已有记录`。
-- Go/No-Go 的形状规则在库里再守一遍：No-Go 必带处理方式且不得保留偏差，Go 反之。
CREATE TABLE pilot_governance.stage_review (
    objective        text        NOT NULL,
    candidate_set_id text        NOT NULL,

    stage            text        NOT NULL,
    scope            text        NOT NULL,
    evidence_pack    text        NOT NULL,
    verdict          text        NOT NULL,
    disposition      text,
    deviations       jsonb       NOT NULL,
    decided_by       text        NOT NULL,
    decided_at       timestamptz NOT NULL,
    effective_at     timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT stage_review_pkey PRIMARY KEY (objective, candidate_set_id),
    CONSTRAINT stage_review_candidates_fkey
        FOREIGN KEY (candidate_set_id)
        REFERENCES pilot_governance.candidate_version_set (set_id),

    CONSTRAINT stage_review_not_blank
        CHECK (
            btrim(objective) <> ''
            AND btrim(candidate_set_id) <> ''
            AND btrim(scope) <> ''
            AND btrim(evidence_pack) <> ''
            AND btrim(decided_by) <> ''
        ),
    CONSTRAINT stage_review_stage_closed
        CHECK (stage IN ('NOT_YET_IN_EXECUTION', 'HISTORICAL_REPLAY', 'SHADOW_RUN', 'LIMITED_PRODUCTION')),
    CONSTRAINT stage_review_verdict_closed
        CHECK (verdict IN ('GO', 'NO_GO')),
    CONSTRAINT stage_review_disposition_shape
        CHECK (
            (verdict = 'GO' AND disposition IS NULL)
            OR (verdict = 'NO_GO' AND disposition IN
                ('KEEP_CURRENT_SCOPE', 'SUSPEND_NEW_ADMISSION', 'FIX_AND_REASSESS', 'OBJECT_LEVEL_TAKEOVER'))
        ),
    CONSTRAINT stage_review_deviations_shape
        CHECK (
            jsonb_typeof(deviations) = 'array'
            AND (verdict = 'GO' OR jsonb_array_length(deviations) = 0)
        )
);

-- 生产权威区间：只追加。重叠预检在应用层（DetectAuthorityConflicts 先于任何落库），
-- 库不重判重叠——但同维完全重复的行由唯一约束拦下兼作幂等（重放补追加不长第二行）。
-- to_at 为 NULL 表示开放区间，NULLS NOT DISTINCT 让「同一开放区间」也只此一行。
CREATE TABLE pilot_governance.authority_interval (
    interval_id  bigint      GENERATED ALWAYS AS IDENTITY,

    object_scope text        NOT NULL,
    capability   text        NOT NULL,
    fact_kind    text        NOT NULL,
    authority    text        NOT NULL,
    from_at      timestamptz NOT NULL,
    to_at        timestamptz,
    inserted_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT authority_interval_pkey PRIMARY KEY (interval_id),
    CONSTRAINT authority_interval_not_blank
        CHECK (
            btrim(object_scope) <> ''
            AND btrim(capability) <> ''
            AND btrim(fact_kind) <> ''
            AND btrim(authority) <> ''
        ),
    CONSTRAINT authority_interval_bounds
        CHECK (to_at IS NULL OR to_at > from_at),
    CONSTRAINT authority_interval_exact_duplicate
        UNIQUE NULLS NOT DISTINCT (object_scope, capability, fact_kind, authority, from_at, to_at)
);
