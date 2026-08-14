-- 后续申报动作、外部舱单引用与关务案件关闭。
--
-- follow_up_target：键=租户+触发依据+提交版本+动作种类。同一触发对同一版本的同类
-- 动作只立一个目标（写入代数同 ADR-0031）。目标形成不等于资料/提交/结果——行内
-- 没有那些列。
--
-- follow_up_replacement：键与目标同一套，外键指回目标——拟替代不可能先于目标存在。
-- 建立与生效分开：SaveRelation 只插拟替代，UpdateRelation 只动生效三列。
--
-- external_manifest_reference：键=租户+外部舱单身份，一舱单一当前引用。Save 接受
-- 首版；Update 推进来源版本（prior_version 指回、关联不自动搬移）。
--
-- case_closure：键=租户+案件引用，一案至多一份关闭记录。义务清单与重开历史进 jsonb；
-- 重开是同记录追加，不翻原关闭列。
--
-- 带 NULL 列的 CHECK 一律走 IS NULL 显式分支。PostgreSQL 不允许 CHECK 含子查询，
-- jsonb 数组逐元封闭集合由领域重建口拦。

CREATE TABLE customs_compliance.follow_up_target (
    tenant_id    text        NOT NULL,
    trigger_ref  text        NOT NULL,
    version_id   text        NOT NULL,
    kind         text        NOT NULL,

    case_ref     text        NOT NULL,
    unit_id      text        NOT NULL,
    scope_ref    text        NOT NULL,
    formed_at    timestamptz NOT NULL,

    CONSTRAINT follow_up_target_pkey
        PRIMARY KEY (tenant_id, trigger_ref, version_id, kind),

    CONSTRAINT follow_up_target_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(trigger_ref) <> ''
            AND btrim(version_id) <> ''
            AND btrim(case_ref) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(scope_ref) <> ''
        ),
    CONSTRAINT follow_up_target_kind_closed
        CHECK (kind IN (
            'IN_CASE_SUPPLEMENT',
            'IN_CASE_CORRECTION',
            'WITHDRAWAL',
            'RESUBMISSION_REPLACEMENT'
        ))
);

CREATE TABLE customs_compliance.follow_up_replacement (
    tenant_id         text        NOT NULL,
    trigger_ref       text        NOT NULL,
    version_id        text        NOT NULL,
    kind              text        NOT NULL,

    replacement_unit  text        NOT NULL,
    effective         boolean     NOT NULL DEFAULT false,
    external_result   text,
    effective_at      timestamptz,

    CONSTRAINT follow_up_replacement_pkey
        PRIMARY KEY (tenant_id, trigger_ref, version_id, kind),

    CONSTRAINT follow_up_replacement_target_fk
        FOREIGN KEY (tenant_id, trigger_ref, version_id, kind)
        REFERENCES customs_compliance.follow_up_target
            (tenant_id, trigger_ref, version_id, kind),

    CONSTRAINT follow_up_replacement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(trigger_ref) <> ''
            AND btrim(version_id) <> ''
            AND btrim(replacement_unit) <> ''
        ),
    -- 拟替代没有外部结果与生效时间；有效替代三者同在场。
    CONSTRAINT follow_up_replacement_effect_shape
        CHECK (
            ((NOT effective) = (external_result IS NULL))
            AND ((NOT effective) = (effective_at IS NULL))
            AND (external_result IS NULL OR btrim(external_result) <> '')
        )
);

CREATE TABLE customs_compliance.external_manifest_reference (
    tenant_id        text        NOT NULL,
    manifest_id      text        NOT NULL,

    version_id       text        NOT NULL,
    carrier_ref      text        NOT NULL,
    procedure_ref    text        NOT NULL,
    direction        text        NOT NULL,
    scope_ref        text        NOT NULL,
    source_fact      text        NOT NULL,
    accepted_at      timestamptz NOT NULL,
    prior_version    text,
    association_unit text,

    CONSTRAINT external_manifest_reference_pkey
        PRIMARY KEY (tenant_id, manifest_id),

    CONSTRAINT external_manifest_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(manifest_id) <> ''
            AND btrim(version_id) <> ''
            AND btrim(carrier_ref) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(source_fact) <> ''
        ),
    CONSTRAINT external_manifest_direction_closed
        CHECK (direction IN ('IMPORT', 'EXPORT')),
    CONSTRAINT external_manifest_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version_id)
        ),
    CONSTRAINT external_manifest_association_not_blank
        CHECK (association_unit IS NULL OR btrim(association_unit) <> '')
);

CREATE TABLE customs_compliance.case_closure (
    tenant_id    text        NOT NULL,
    case_ref     text        NOT NULL,

    cutoff_at    timestamptz NOT NULL,
    verified_at  timestamptz NOT NULL,
    decided_by   text        NOT NULL,
    closed_at    timestamptz NOT NULL,
    items        jsonb       NOT NULL,
    reopenings   jsonb       NOT NULL DEFAULT '[]'::jsonb,

    CONSTRAINT case_closure_pkey PRIMARY KEY (tenant_id, case_ref),

    CONSTRAINT case_closure_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(case_ref) <> ''
            AND btrim(decided_by) <> ''
        ),
    CONSTRAINT case_closure_time_order
        CHECK (closed_at >= verified_at),
    CONSTRAINT case_closure_items_present
        CHECK (jsonb_typeof(items) = 'array' AND jsonb_array_length(items) >= 1),
    CONSTRAINT case_closure_reopenings_shaped
        CHECK (jsonb_typeof(reopenings) = 'array')
);
