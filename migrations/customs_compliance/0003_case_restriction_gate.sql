-- 关务案件、监管限制与放行门禁核对。
--
-- customs_case：键=租户+固定监管范围四维（辖区+方向+程序+义务范围）——同一法律行为
-- 一案，同一包裹进入另一独立监管程序自然换键。案件是责任容器不是状态机：行内没有
-- 状态推进列，无 UPDATE 语句。包裹关联与角色快照存 jsonb——只引用不复制源事实，
-- 无按成员检索的读面。
--
-- regulatory_restriction：键=租户+限制标识。建立与解除分开：建立走 ON CONFLICT
-- 代数，解除是同一限制的状态推进（Update 只动解除两列——约束集与范围在建立时固定）。
-- 解除只凭责任来源接受的监管结果：released_at 非空必须 released_by 同在场。
--
-- gate_verification：键=租户+判断身份三维（范围+动作+边界）+逐项判断指纹——条件
-- 状态变化自然换指纹换版，同一状态重复核对不出第二版。行内没有放行列：门禁满足
-- 不生成放行（CONTEXT 硬句 216 半边）。
--
-- 带 NULL 列的 CHECK 一律走 IS NULL 显式分支（会话纪律：普通比较在 NULL 上会让
-- 整条约束按 NULL 放行）。

CREATE TABLE customs_compliance.customs_case (
    tenant_id        text        NOT NULL,
    jurisdiction_ref text        NOT NULL,
    direction        text        NOT NULL,
    procedure_ref    text        NOT NULL,
    obligation_ref   text        NOT NULL,

    case_id          text        NOT NULL,
    parcels          jsonb       NOT NULL,
    roles            jsonb       NOT NULL,
    established_at   timestamptz NOT NULL,

    CONSTRAINT customs_case_pkey
        PRIMARY KEY (tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref),
    CONSTRAINT customs_case_id_unique UNIQUE (tenant_id, case_id),

    CONSTRAINT customs_case_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(jurisdiction_ref) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(obligation_ref) <> ''
            AND btrim(case_id) <> ''
        ),
    CONSTRAINT customs_case_direction_closed
        CHECK (direction IN ('IMPORT', 'EXPORT')),
    -- 包裹关联集缺一不可；角色快照可为空清单（初始角色未确认如实空白）。
    CONSTRAINT customs_case_parcels_present
        CHECK (jsonb_typeof(parcels) = 'array' AND jsonb_array_length(parcels) >= 1),
    CONSTRAINT customs_case_roles_shaped
        CHECK (jsonb_typeof(roles) = 'array')
);

CREATE TABLE customs_compliance.regulatory_restriction (
    tenant_id      text        NOT NULL,
    restriction_id text        NOT NULL,

    decision_id    text        NOT NULL,
    scope_ref      text        NOT NULL,
    constrains     jsonb       NOT NULL,
    effective_at   timestamptz NOT NULL,
    released_by    text,
    released_at    timestamptz,

    CONSTRAINT regulatory_restriction_pkey PRIMARY KEY (tenant_id, restriction_id),

    CONSTRAINT regulatory_restriction_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(restriction_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(scope_ref) <> ''
        ),
    -- 约束集必须显式声明且非空——「阻断其明确约束的」动作，没列的动作不受它管。
    CONSTRAINT regulatory_restriction_constrains_present
        CHECK (jsonb_typeof(constrains) = 'array' AND jsonb_array_length(constrains) >= 1),
    -- 解除只凭监管结果引用：依据与时刻同在场，时刻不早于生效。
    CONSTRAINT regulatory_restriction_release_shape
        CHECK (
            ((released_by IS NULL) = (released_at IS NULL))
            AND (released_by IS NULL OR btrim(released_by) <> '')
            AND (released_at IS NULL OR released_at >= effective_at)
        )
);

-- ListByScope 是准入判断的读面：按（租户+范围）盘出全部限制——仍有效与否由领域
-- 判（JudgeActionAdmissibility 自己跳过已解除的），库不预筛。
CREATE INDEX regulatory_restriction_by_scope
    ON customs_compliance.regulatory_restriction (tenant_id, scope_ref, effective_at, restriction_id);

CREATE TABLE customs_compliance.gate_verification (
    tenant_id       text        NOT NULL,
    scope_ref       text        NOT NULL,
    action          text        NOT NULL,
    boundary_ref    text        NOT NULL,
    findings_digest text        NOT NULL,

    preconditions   jsonb       NOT NULL,
    conclusion      text        NOT NULL,
    verified_at     timestamptz NOT NULL,

    CONSTRAINT gate_verification_pkey
        PRIMARY KEY (tenant_id, scope_ref, action, boundary_ref, findings_digest),

    CONSTRAINT gate_verification_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(boundary_ref) <> ''
            AND btrim(findings_digest) <> ''
        ),
    -- 受门禁约束的方向性动作封闭四值（CONTEXT 硬句逐词）。
    CONSTRAINT gate_verification_action_closed
        CHECK (action IN ('OUTBOUND_RELEASE', 'LOADING_DEPARTURE', 'CROSS_CUSTOMS_MOVEMENT', 'FINAL_DELIVERY')),
    -- 门禁结论封闭五值（CONTEXT「放行门禁核对」语言逐词）。
    CONSTRAINT gate_verification_conclusion_closed
        CHECK (conclusion IN ('UNMET', 'PARTIALLY_MET', 'MET', 'CONFLICTING', 'NOT_APPLICABLE')),
    -- 说不出核对了哪些前置条件的门禁判断无从复核；只有`不适用`可以没有前置条件。
    CONSTRAINT gate_verification_preconditions_shape
        CHECK (
            jsonb_typeof(preconditions) = 'array'
            AND (conclusion = 'NOT_APPLICABLE' OR jsonb_array_length(preconditions) >= 1)
        )
);
