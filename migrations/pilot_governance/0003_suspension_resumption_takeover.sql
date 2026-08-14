-- 暂停决定、恢复决定与对象级受控接管。治理是产品级机制，没有租户维；只存脱敏引用。
-- 0001 已有候选组/评审/权威区间，本文件不重做那三口。
--
-- suspension_decision：按暂停标识，不可覆盖。
-- resumption_decision：按被恢复的暂停标识，一暂停至多一次恢复；外键指回暂停——
-- 恢复不可能先于暂停存在。再暂停是新的暂停决定。
-- takeover_record：按区间四维身份（对象范围×能力×事实类型×权威方）加生效区间
-- 定位；同维完全重复由 UNIQUE NULLS NOT DISTINCT 拦下兼作幂等。
--
-- 在途盘点进 jsonb。带 NULL 列的 CHECK 走 IS NULL 显式分支。

CREATE TABLE pilot_governance.suspension_decision (
    suspension_id   text        NOT NULL,
    trigger_source  text        NOT NULL,
    basis           text        NOT NULL,
    evidence        text        NOT NULL,
    scope           text        NOT NULL,
    executed_by     text        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    effective_at    timestamptz NOT NULL,
    in_transit_note text        NOT NULL,

    CONSTRAINT suspension_decision_pkey PRIMARY KEY (suspension_id),
    CONSTRAINT suspension_decision_not_blank
        CHECK (
            btrim(suspension_id) <> ''
            AND btrim(trigger_source) <> ''
            AND btrim(basis) <> ''
            AND btrim(evidence) <> ''
            AND btrim(scope) <> ''
            AND btrim(executed_by) <> ''
            AND btrim(in_transit_note) <> ''
        )
);

CREATE TABLE pilot_governance.resumption_decision (
    suspension_id      text        NOT NULL,
    release_evidence   text        NOT NULL,
    consistency_check  text        NOT NULL,
    inventory          jsonb       NOT NULL,
    inventory_taken_at timestamptz NOT NULL,
    decided_by         text        NOT NULL,
    decided_at         timestamptz NOT NULL,
    effective_at       timestamptz NOT NULL,

    CONSTRAINT resumption_decision_pkey PRIMARY KEY (suspension_id),
    CONSTRAINT resumption_decision_suspension_fk
        FOREIGN KEY (suspension_id)
        REFERENCES pilot_governance.suspension_decision (suspension_id),

    CONSTRAINT resumption_decision_not_blank
        CHECK (
            btrim(suspension_id) <> ''
            AND btrim(release_evidence) <> ''
            AND btrim(consistency_check) <> ''
            AND btrim(decided_by) <> ''
        ),
    CONSTRAINT resumption_decision_inventory_present
        CHECK (jsonb_typeof(inventory) = 'array' AND jsonb_array_length(inventory) >= 1)
);

CREATE TABLE pilot_governance.takeover_record (
    object_scope      text        NOT NULL,
    capability        text        NOT NULL,
    fact_kind         text        NOT NULL,
    authority         text        NOT NULL,
    from_at           timestamptz NOT NULL,
    to_at             timestamptz,

    stop_evidence     text        NOT NULL,
    accepted_facts    text        NOT NULL,
    pending_externals text        NOT NULL,
    actual_control    text        NOT NULL,
    responsibilities  text        NOT NULL,
    next_action       text        NOT NULL,
    inventory         jsonb       NOT NULL,
    inventory_taken_at timestamptz NOT NULL,
    effective_at      timestamptz NOT NULL,

    CONSTRAINT takeover_record_not_blank
        CHECK (
            btrim(object_scope) <> ''
            AND btrim(capability) <> ''
            AND btrim(fact_kind) <> ''
            AND btrim(authority) <> ''
            AND btrim(stop_evidence) <> ''
            AND btrim(accepted_facts) <> ''
            AND btrim(pending_externals) <> ''
            AND btrim(actual_control) <> ''
            AND btrim(responsibilities) <> ''
            AND btrim(next_action) <> ''
        ),
    CONSTRAINT takeover_record_bounds
        CHECK (to_at IS NULL OR to_at > from_at),
    CONSTRAINT takeover_record_inventory_present
        CHECK (jsonb_typeof(inventory) = 'array' AND jsonb_array_length(inventory) >= 1),
    CONSTRAINT takeover_record_interval_identity
        UNIQUE NULLS NOT DISTINCT (object_scope, capability, fact_kind, authority, from_at, to_at)
);
