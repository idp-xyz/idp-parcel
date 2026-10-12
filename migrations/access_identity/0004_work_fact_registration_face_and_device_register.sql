-- 「作业事实登记」能力面与作业设备册（ADR-0149 决定二；票 operator-channel/10）：一线人员登作业事实的授予按租户 ×
-- 作业范围登记在操作者册里，与既有各格并列；设备单列一册，记设备标识、所属租户与作业范围、停用。与
-- accessidentity.CapabilityWorkFactRegistration、accessidentity.WorkScopeKind 同笔放开（0001 头注那条约定）。
-- 本迁移不种任何行：授予随参数登记册 PAR-INT-08、设备随 PAR-INT-10，都是租户取值。
--
-- 作业范围是授予的两列，不是另一张表，理由同 0002 的决定种类：撤了再授仍是两笔授予、各带自己的范围。

ALTER TABLE access_identity.operator_grant
    ADD COLUMN work_scope_kind text,
    ADD COLUMN work_scope_ref  text;

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_capability_face_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_capability_face_decided
        CHECK (capability_face IN (
            'REGISTRY_CONFIGURATION_WRITE', 'MASTER_DATA_AND_OPERATIONS_READ', 'OPERATION_DECISION',
            'WORK_FACT_REGISTRATION'
        ));

-- 「作业事实登记」一格必带作业范围，别的格必不带：不带范围的一笔等于把所有节点一并授出，带了范围的登记册配置写则是
-- 一行说不清在授什么的授予。种类与引用同在同缺。
ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_work_scope_matches_face
        CHECK (
            ((capability_face = 'WORK_FACT_REGISTRATION') = (work_scope_kind IS NOT NULL))
            AND ((work_scope_kind IS NULL) = (work_scope_ref IS NULL))
        );

-- 与 accessidentity.WorkScopeKind 同一份封闭集，两处同笔改。
ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_work_scope_decided
        CHECK (work_scope_kind IS NULL OR (work_scope_kind IN ('NODE', 'OFFSITE') AND btrim(work_scope_ref) <> ''));

CREATE TABLE access_identity.device (
    tenant_id       text        NOT NULL,
    device_id       text        NOT NULL,
    work_scope_kind text        NOT NULL,
    work_scope_ref  text        NOT NULL,
    basis_ref       text        NOT NULL,
    recorded_at     timestamptz NOT NULL DEFAULT now(),

    -- 键含租户：设备标识由租户登记时给出，不像操作者主体那样由发行方全局签发；别的租户登的同名设备是另一台。
    CONSTRAINT device_pkey PRIMARY KEY (tenant_id, device_id),

    CONSTRAINT device_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(device_id) <> ''
            AND btrim(work_scope_ref) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 与 accessidentity.WorkScopeKind 同一份封闭集，两处同笔改。
    CONSTRAINT device_work_scope_decided
        CHECK (work_scope_kind IN ('NODE', 'OFFSITE'))
);

CREATE TABLE access_identity.device_deactivation (
    tenant_id      text        NOT NULL,
    device_id      text        NOT NULL,
    deactivated_at timestamptz NOT NULL,
    basis_ref      text        NOT NULL,
    recorded_at    timestamptz NOT NULL DEFAULT now(),

    -- 一台设备至多停一次：停用是终局，换机或重置要登新的设备标识（ADR-0023 的设备身份内序号单调）。
    CONSTRAINT device_deactivation_pkey PRIMARY KEY (tenant_id, device_id),

    CONSTRAINT device_deactivation_device_fkey
        FOREIGN KEY (tenant_id, device_id)
        REFERENCES access_identity.device (tenant_id, device_id),

    CONSTRAINT device_deactivation_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);
