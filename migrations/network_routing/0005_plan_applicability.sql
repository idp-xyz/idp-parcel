-- 计划适用性另立记录：计划本体在 initial_route 里不可变，适用性按计划版本单独存。
-- 主键取计划版本——签发方保证版本标识全局唯一；同一计划的适用性一行，Save 换态
-- （当前有效 → 已被替代/已失效/已结束），不另开行。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

CREATE TABLE network_routing.plan_applicability (
    plan_id          text        NOT NULL,
    state            text        NOT NULL,
    transitioned_at  timestamptz NOT NULL,
    basis            text,
    successor        text,
    recorded_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT plan_applicability_pkey
        PRIMARY KEY (plan_id),

    CONSTRAINT plan_applicability_scope_not_blank
        CHECK (
            btrim(plan_id) <> ''
            AND btrim(state) <> ''
        ),

    CONSTRAINT plan_applicability_state_closed
        CHECK (state IN
            ('CURRENTLY_EFFECTIVE', 'SUPERSEDED', 'LAPSED', 'CONCLUDED')),

    -- 当前有效无依据无接班；已被替代必须两者都在且接班不是自己；失效/结束必须有
    -- 依据、不得有接班。可空列先验 IS NULL / IS NOT NULL。
    CONSTRAINT plan_applicability_state_shaped
        CHECK (
            (state = 'CURRENTLY_EFFECTIVE'
             AND basis IS NULL AND successor IS NULL)
            OR (state = 'SUPERSEDED'
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND successor IS NOT NULL AND btrim(successor) <> ''
                AND successor <> plan_id)
            OR (state IN ('LAPSED', 'CONCLUDED')
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND successor IS NULL)
        )
);
