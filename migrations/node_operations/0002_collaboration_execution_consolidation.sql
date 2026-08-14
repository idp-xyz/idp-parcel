-- 承接决定、执行事实、合箱单元与当前容纳索引。
--
-- collaboration_acceptance 主键取（租户+协作事项）：同一事项只决定一次，同键第二份
-- 由主键拦住，适配器以 ON CONFLICT DO NOTHING 译成`已有决定`。三值各自的在场件
-- （接受无依据有范围、拒接有依据无范围、部分两者都有）由 CHECK 钉住。
--
-- execution_fact 主键取（租户+事项+实物+动作）：同一格只登一次。动作封闭五值。
--
-- consolidation_unit 主键取（租户+实例）：Save 只管开启；成员/封装/关闭走 Update。
-- 当前成员与历史快照都在行上——关闭后成员仍可能留在实例里（处置转移），但不再
-- 占据当前父级。
--
-- containment_current 是「同一时点至多一个直接物理父级」的库面：只收录未关闭单元
-- 的当前成员，主键（租户+实物）即单父级。关闭时适配器删行，实物才能进入下一个单元。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取长度——jsonb_array_length(NULL)
-- 是 NULL，会让整条按 NULL 放行。

CREATE TABLE node_operations.collaboration_acceptance (
    tenant_id         text        NOT NULL,
    item_ref          text        NOT NULL,

    node_ref          text        NOT NULL,
    decision          text        NOT NULL,
    authority         text        NOT NULL,
    basis             text,
    accepted_units    jsonb       NOT NULL DEFAULT '[]'::jsonb,
    accepted_actions  jsonb       NOT NULL DEFAULT '[]'::jsonb,
    decided_at        timestamptz NOT NULL,
    content_digest    text        NOT NULL,
    recorded_at       timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT collaboration_acceptance_pkey
        PRIMARY KEY (tenant_id, item_ref),

    CONSTRAINT collaboration_acceptance_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(item_ref) <> ''
            AND btrim(node_ref) <> ''
            AND btrim(authority) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT collaboration_acceptance_decision_closed
        CHECK (decision IN ('ACCEPTED', 'DECLINED', 'PARTIALLY_ACCEPTED')),

    -- jsonb_typeof(NULL) 是 NULL，单独 typeof=array 会放行缺列。
    CONSTRAINT collaboration_acceptance_lists_shaped
        CHECK (
            accepted_units IS NOT NULL AND jsonb_typeof(accepted_units) = 'array'
            AND accepted_actions IS NOT NULL AND jsonb_typeof(accepted_actions) = 'array'
        ),

    CONSTRAINT collaboration_acceptance_decision_shape
        CHECK (
            (decision = 'ACCEPTED'
                AND basis IS NULL
                AND jsonb_array_length(accepted_units) > 0
                AND jsonb_array_length(accepted_actions) > 0)
            OR (decision = 'DECLINED'
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND jsonb_array_length(accepted_units) = 0
                AND jsonb_array_length(accepted_actions) = 0)
            OR (decision = 'PARTIALLY_ACCEPTED'
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND jsonb_array_length(accepted_units) > 0
                AND jsonb_array_length(accepted_actions) > 0)
        )
);

CREATE TABLE node_operations.execution_fact (
    tenant_id      text        NOT NULL,
    item_ref       text        NOT NULL,
    unit_id        text        NOT NULL,
    action         text        NOT NULL,

    node_ref       text        NOT NULL,
    evidence       text        NOT NULL,
    performed_at   timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT execution_fact_pkey
        PRIMARY KEY (tenant_id, item_ref, unit_id, action),

    CONSTRAINT execution_fact_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(item_ref) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(node_ref) <> ''
            AND btrim(evidence) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT execution_fact_action_closed
        CHECK (action IN ('UNSEAL', 'ISOLATE', 'PRESENT', 'TALLY', 'OBSERVE'))
);

CREATE TABLE node_operations.consolidation_unit (
    tenant_id   text        NOT NULL,
    unit_id     text        NOT NULL,

    asset_ref   text        NOT NULL,
    phase       text        NOT NULL,
    members     jsonb       NOT NULL DEFAULT '[]'::jsonb,
    snapshots   jsonb       NOT NULL DEFAULT '[]'::jsonb,
    closed_at   timestamptz,
    inserted_at timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT consolidation_unit_pkey
        PRIMARY KEY (tenant_id, unit_id),

    CONSTRAINT consolidation_unit_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(asset_ref) <> ''
        ),

    CONSTRAINT consolidation_unit_phase_closed
        CHECK (phase IN ('OPEN', 'SEALED', 'CLOSED')),

    CONSTRAINT consolidation_unit_lists_shaped
        CHECK (
            members IS NOT NULL AND jsonb_typeof(members) = 'array'
            AND snapshots IS NOT NULL AND jsonb_typeof(snapshots) = 'array'
        ),

    -- 关闭与时刻同在或同缺；等号碰上 NULL 给 NULL，必须先 IS NULL / IS NOT NULL。
    CONSTRAINT consolidation_unit_closed_at_coupled
        CHECK (
            (phase = 'CLOSED' AND closed_at IS NOT NULL)
            OR (phase IN ('OPEN', 'SEALED') AND closed_at IS NULL)
        ),

    CONSTRAINT consolidation_unit_sealed_has_snapshot
        CHECK (
            phase <> 'SEALED'
            OR jsonb_array_length(snapshots) > 0
        )
);

CREATE TABLE node_operations.containment_current (
    tenant_id text NOT NULL,
    member_id text NOT NULL,
    unit_id   text NOT NULL,

    CONSTRAINT containment_current_pkey
        PRIMARY KEY (tenant_id, member_id),

    CONSTRAINT containment_current_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(member_id) <> ''
            AND btrim(unit_id) <> ''
        ),

    CONSTRAINT containment_current_unit_fk
        FOREIGN KEY (tenant_id, unit_id)
        REFERENCES node_operations.consolidation_unit (tenant_id, unit_id)
);

CREATE INDEX containment_current_by_unit
    ON node_operations.containment_current (tenant_id, unit_id);
