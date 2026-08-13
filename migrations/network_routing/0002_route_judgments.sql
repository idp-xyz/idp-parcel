-- 初始路由判断与复核记录：UC-NR-001 的两张判断库。
--
-- initial_route 主键取完整判断键（租户+客户+请求+接受基线+包裹+服务目的）：「同一
-- 接受基线、同一包裹和同一初始路由目的只能形成一个当前有效初始路由结果」由键的选维
-- 承担；撞键即`已有记录`（ON CONFLICT 代数，ADR-0031），不覆盖先到者。计划与无路
-- 可走判断二居其一（XOR CHECK）——「无路由不得用空计划表达」的库面。
--
-- route_reassessment 主键取（租户+触发关联）：同一触发和输入版本只处理一次，重复
-- 触发按关联读回原结果。四种领域走向的在场件矩阵由逐结论 CHECK 钉住——一行说
-- `仍适用`却带失效依据，写进来就是写入方的缺陷。

CREATE TABLE network_routing.initial_route (
    tenant_id            text        NOT NULL,
    customer_account_id  text        NOT NULL,
    shipment_request_id  text        NOT NULL,
    acceptance_baseline  text        NOT NULL,
    declared_parcel_id   text        NOT NULL,
    service_purpose      text        NOT NULL,

    conclusion           text        NOT NULL,
    plan_version         text,
    plan                 jsonb,
    no_route             jsonb,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT initial_route_pkey
        PRIMARY KEY (tenant_id, customer_account_id, shipment_request_id,
                     acceptance_baseline, declared_parcel_id, service_purpose),

    CONSTRAINT initial_route_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
        ),

    CONSTRAINT initial_route_conclusion_closed
        CHECK (conclusion IN ('ROUTE_FORMED', 'NO_CURRENT_ROUTE')),

    -- 计划与无路可走二居其一；计划必有版本号——两个都带或都缺的行是坏数据
    -- （ports.InitialRouteRecord 注释的存储面）。
    CONSTRAINT initial_route_exactly_one_result
        CHECK (
            (conclusion = 'ROUTE_FORMED'
                AND plan IS NOT NULL
                AND plan_version IS NOT NULL AND btrim(plan_version) <> ''
                AND no_route IS NULL)
            OR (conclusion = 'NO_CURRENT_ROUTE'
                AND no_route IS NOT NULL
                AND plan IS NULL
                AND plan_version IS NULL)
        )
);

CREATE TABLE network_routing.route_reassessment (
    tenant_id            text        NOT NULL,
    correlation_id       text        NOT NULL,

    customer_account_id  text        NOT NULL,
    shipment_request_id  text        NOT NULL,
    acceptance_baseline  text        NOT NULL,
    declared_parcel_id   text        NOT NULL,
    service_purpose      text        NOT NULL,

    conclusion           text        NOT NULL,
    reviewed_plan        text,
    lapse_basis          text,
    candidate_state      text,
    reroute_state        text,
    reroute_blockers     jsonb,
    new_plan             jsonb,
    suggestion           jsonb,
    decision             jsonb,
    reassessed_at        timestamptz NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT route_reassessment_pkey
        PRIMARY KEY (tenant_id, correlation_id),

    CONSTRAINT route_reassessment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(correlation_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
        ),

    -- 四种领域走向在库里也封闭（ports.ReassessmentConclusionKind）。
    CONSTRAINT route_reassessment_conclusion_closed
        CHECK (conclusion IN
            ('STILL_APPLICABLE', 'PLAN_LAPSED', 'FIRST_PLAN_FORMED', 'REROUTED')),

    -- 候选评估状态封闭三值；NULL 表示本走向不评估（仅`仍适用`）。IS NOT NULL 先行
    -- 再 IN：SQL 三值逻辑下 NULL IN (...) 是 NULL，整条 CHECK 会按 NULL 放行。
    CONSTRAINT route_reassessment_candidate_state_closed
        CHECK (
            (conclusion = 'STILL_APPLICABLE' AND candidate_state IS NULL)
            OR (conclusion <> 'STILL_APPLICABLE'
                AND candidate_state IS NOT NULL
                AND candidate_state IN
                ('CANDIDATES_AVAILABLE', 'NO_QUALIFIED_CANDIDATES', 'CANDIDATE_REVIEW_UNDECIDED'))
        ),

    -- 改路判定封闭三态；NULL 表示没评估（事实目录未配置或候选未收敛）。
    CONSTRAINT route_reassessment_reroute_state_closed
        CHECK (reroute_state IS NULL OR reroute_state IN
            ('AUTOMATIC_ALLOWED', 'SUGGESTION_ONLY', 'BARRED')),

    -- 改路评估只在失效的原计划上有意义：判定在场必有被复核计划。
    CONSTRAINT route_reassessment_reroute_on_reviewed_plan
        CHECK (reroute_state IS NULL OR reviewed_plan IS NOT NULL),

    -- 判定与其携带物互证：建议是「没自动成」的产物、阻塞清单说得出为什么没自动，
    -- `已允许`则两者都不该有（自动成了是决定、没成是决定缺席）。可空列先验
    -- IS NOT NULL 再取长度：jsonb_array_length(NULL) 是 NULL，会让整条按 NULL 放行。
    CONSTRAINT route_reassessment_reroute_coupling
        CHECK (
            (reroute_state IS NULL AND reroute_blockers IS NULL AND suggestion IS NULL)
            OR (reroute_state = 'AUTOMATIC_ALLOWED'
                AND reroute_blockers IS NULL AND suggestion IS NULL)
            OR (reroute_state = 'SUGGESTION_ONLY'
                AND reroute_blockers IS NOT NULL
                AND jsonb_array_length(reroute_blockers) > 0)
            OR (reroute_state = 'BARRED'
                AND reroute_blockers IS NOT NULL
                AND jsonb_array_length(reroute_blockers) > 0
                AND suggestion IS NULL)
        ),

    -- 逐结论在场件矩阵（reassess_route.go 四条提交路的库面）。
    CONSTRAINT route_reassessment_still_applicable_shape
        CHECK (conclusion <> 'STILL_APPLICABLE' OR (
            reviewed_plan IS NOT NULL
            AND lapse_basis IS NULL AND new_plan IS NULL
            AND suggestion IS NULL AND decision IS NULL AND reroute_state IS NULL)),
    CONSTRAINT route_reassessment_first_plan_shape
        CHECK (conclusion <> 'FIRST_PLAN_FORMED' OR (
            new_plan IS NOT NULL
            AND reviewed_plan IS NULL AND lapse_basis IS NULL
            AND suggestion IS NULL AND decision IS NULL AND reroute_state IS NULL)),
    -- reroute_state 可空，等号在 NULL 上给 NULL；IS NOT DISTINCT FROM 给确定的假。
    CONSTRAINT route_reassessment_rerouted_shape
        CHECK (conclusion <> 'REROUTED' OR (
            reviewed_plan IS NOT NULL AND lapse_basis IS NOT NULL
            AND new_plan IS NOT NULL AND decision IS NOT NULL
            AND suggestion IS NULL
            AND reroute_state IS NOT DISTINCT FROM 'AUTOMATIC_ALLOWED')),
    -- `已失效`分两路：原有计划失效（被复核计划与失效依据同在）；原本就无路由的
    -- 复核未成计划（两者同缺，也不该有改路痕迹——没有失效的计划就没有改路评估）。
    CONSTRAINT route_reassessment_lapsed_shape
        CHECK (conclusion <> 'PLAN_LAPSED' OR (
            new_plan IS NULL AND decision IS NULL
            AND ((reviewed_plan IS NOT NULL AND lapse_basis IS NOT NULL)
                 OR (reviewed_plan IS NULL AND lapse_basis IS NULL
                     AND suggestion IS NULL AND reroute_state IS NULL))))
);
