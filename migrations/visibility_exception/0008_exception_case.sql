-- 异常案件：键=租户+案件标识。本表是 ExceptionCase 聚合的落点，列与聚合字段一一
-- 对应；`ActiveCaseView` 的在场判据（处置请求只能挂在活案件下）读的就是 phase。
-- 租户是最高数据隔离边界（ADR-0003）——案件标识只在租户内唯一。
--
-- 本轮只落表与只读在场判据，不落案件仓储：ExceptionCase 尚无快照/重建构造器，补一个
-- 属领域改动，不在本轮范围。表按聚合的完整形状建，是为了让将来那个写入方无法存进
-- 一个半截案件——只够回答「活没活」的窄表反而会把不变量留在库外。

CREATE TABLE visibility_exception.exception_case (
    tenant_id        text        NOT NULL,
    case_id          text        NOT NULL,

    root_parcel      text        NOT NULL,
    impact_scope     text        NOT NULL,
    responsible_team text        NOT NULL,
    phase            text        NOT NULL,
    established_at   timestamptz NOT NULL,
    first_response   timestamptz,
    closed_at        timestamptz,
    conclusion       text,
    merged_into      text,

    CONSTRAINT exception_case_pkey PRIMARY KEY (tenant_id, case_id),

    CONSTRAINT exception_case_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(case_id) <> ''
            AND btrim(root_parcel) <> ''
            AND btrim(impact_scope) <> ''
            AND btrim(responsible_team) <> ''
        ),

    -- 精简主生命周期三态（CONTEXT 硬句）。等待客户、监控中、已升级是工作条件，
    -- 刻意不在此枚举——库里多一格就等于默许它们扩成互斥主状态。
    CONSTRAINT exception_case_phase_closed_set
        CHECK (phase IN ('AWAITING_RESPONSE', 'IN_PROGRESS', 'CLOSED')),

    -- 已关闭必带结论与关闭时间；未关闭一律不带。关闭要「保存结论、依据、影响范围
    -- 和未决事项」，一个没有结论的已关闭案件在业务上不成立。
    CONSTRAINT exception_case_closed_carries_conclusion
        CHECK (
            (phase = 'CLOSED') = (closed_at IS NOT NULL)
            AND (closed_at IS NOT NULL) = (conclusion IS NOT NULL)
            AND (conclusion IS NULL OR btrim(conclusion) <> '')
        ),

    -- 接单形成首次响应时间：处理中必有它。待响应必无——首次响应时间随接单形成，
    -- 没接单就有响应时间会让响应周期绩效凭空好看一截。
    -- 已关闭两可：待响应直接关闭（判定不成立）从不经过接单。
    CONSTRAINT exception_case_first_response_follows_take_up
        CHECK (
            (phase = 'IN_PROGRESS' AND first_response IS NOT NULL)
            OR (phase = 'AWAITING_RESPONSE' AND first_response IS NULL)
            OR phase = 'CLOSED'
        ),

    -- 归并是关闭的一种，且只指向别的案件。自己归并进自己会让主案件链成环，
    -- 而「指定主案件继续处置」要求链有终点。
    CONSTRAINT exception_case_merged_is_closed_elsewhere
        CHECK (
            merged_into IS NULL
            OR (
                phase = 'CLOSED'
                AND btrim(merged_into) <> ''
                AND merged_into <> case_id
            )
        ),

    -- 时间不早于建立。案件历史不得倒填——「后续新增受影响对象按其被确认受影响的
    -- 时间记录，不倒填为事故最初发生时间」同一条纪律。
    CONSTRAINT exception_case_times_not_before_established
        CHECK (
            (first_response IS NULL OR first_response >= established_at)
            AND (closed_at IS NULL OR closed_at >= established_at)
        )
);
