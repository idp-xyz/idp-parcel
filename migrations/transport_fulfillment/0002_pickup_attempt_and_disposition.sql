-- 揽收尝试提交与监管处置承接决定。
--
-- pickup_attempt 主键取（租户+来源身份）：同一来源只提交一次，同键第二份由主键拦住，
-- 适配器以 ON CONFLICT DO NOTHING 译成`已有记录`。失败对象只出现在 result 子表；
-- pickup 子表通过（对象+outcome='PICKED_UP'）外键接到 result——失败行接不上揽收，
-- 「失败结果不制造实际履约段」在库面就成立。
--
-- disposition_acceptance 主键取（租户+协作事项）：一事项一决定，同键异内容冲突不顶替。
-- 三格在场件（接受无拒因有对象与授权、拒接有拒因无对象无授权、部分两半都有）由 CHECK
-- 钉住。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取长度。

CREATE TABLE transport_fulfillment.pickup_attempt (
    tenant_id        text        NOT NULL,
    source_id        text        NOT NULL,

    attempt_ref      text        NOT NULL,
    task_ref         text        NOT NULL,
    executed_by      text        NOT NULL,
    place_ref        text        NOT NULL,
    planned_from     timestamptz NOT NULL,
    planned_to       timestamptz NOT NULL,
    arrived_at       timestamptz NOT NULL,
    evidence_ref     text        NOT NULL,
    rescheduled_from text,
    objects          jsonb       NOT NULL,
    content_digest   text        NOT NULL,
    recorded_at      timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pickup_attempt_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT pickup_attempt_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(executed_by) <> ''
            AND btrim(place_ref) <> ''
            AND btrim(evidence_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT pickup_attempt_objects_shaped
        CHECK (
            objects IS NOT NULL
            AND jsonb_typeof(objects) = 'array'
            AND jsonb_array_length(objects) > 0
        ),

    CONSTRAINT pickup_attempt_window_ordered
        CHECK (planned_to > planned_from),

    -- IS DISTINCT FROM 在 NULL 上给确定的真，不会把「没有前尝试」误判成自指。
    CONSTRAINT pickup_attempt_reschedule_not_self
        CHECK (
            rescheduled_from IS NULL
            OR (btrim(rescheduled_from) <> ''
                AND rescheduled_from IS DISTINCT FROM attempt_ref)
        )
);

CREATE TABLE transport_fulfillment.pickup_attempt_result (
    tenant_id   text        NOT NULL,
    source_id   text        NOT NULL,
    object_ref  text        NOT NULL,

    outcome     text        NOT NULL,
    basis       text,
    occurred_at timestamptz NOT NULL,

    CONSTRAINT pickup_attempt_result_pkey
        PRIMARY KEY (tenant_id, source_id, object_ref),

    CONSTRAINT pickup_attempt_result_outcome_unique
        UNIQUE (tenant_id, source_id, object_ref, outcome),

    CONSTRAINT pickup_attempt_result_attempt_fk
        FOREIGN KEY (tenant_id, source_id)
        REFERENCES transport_fulfillment.pickup_attempt (tenant_id, source_id),

    CONSTRAINT pickup_attempt_result_object_not_blank
        CHECK (btrim(object_ref) <> ''),

    CONSTRAINT pickup_attempt_result_outcome_closed
        CHECK (outcome IN (
            'PICKED_UP', 'CUSTOMER_ABSENT', 'GOODS_NOT_READY', 'PACKAGING_UNACCEPTABLE'
        )),

    -- 成功不得带失败依据；失败必须带因。basis 可空，先验 IS NULL / IS NOT NULL。
    CONSTRAINT pickup_attempt_result_outcome_shape
        CHECK (
            (outcome = 'PICKED_UP' AND basis IS NULL)
            OR (outcome IN ('CUSTOMER_ABSENT', 'GOODS_NOT_READY', 'PACKAGING_UNACCEPTABLE')
                AND basis IS NOT NULL AND btrim(basis) <> '')
        )
);

CREATE TABLE transport_fulfillment.pickup_attempt_pickup (
    tenant_id   text        NOT NULL,
    source_id   text        NOT NULL,
    object_ref  text        NOT NULL,
    outcome     text        NOT NULL DEFAULT 'PICKED_UP',

    task_ref    text        NOT NULL,
    attempt_ref text        NOT NULL,
    place_ref   text        NOT NULL,
    control_ref text        NOT NULL,
    executed_by text        NOT NULL,
    version     text        NOT NULL,
    occurred_at timestamptz NOT NULL,

    CONSTRAINT pickup_attempt_pickup_pkey
        PRIMARY KEY (tenant_id, source_id, object_ref),

    CONSTRAINT pickup_attempt_pickup_picked_up_only
        CHECK (outcome = 'PICKED_UP'),

    CONSTRAINT pickup_attempt_pickup_not_blank
        CHECK (
            btrim(object_ref) <> ''
            AND btrim(task_ref) <> ''
            AND btrim(attempt_ref) <> ''
            AND btrim(place_ref) <> ''
            AND btrim(control_ref) <> ''
            AND btrim(executed_by) <> ''
            AND btrim(version) <> ''
        ),

    -- 只能接到成功结果：失败行的 outcome 对不上 PICKED_UP，外键接不上。
    CONSTRAINT pickup_attempt_pickup_result_fk
        FOREIGN KEY (tenant_id, source_id, object_ref, outcome)
        REFERENCES transport_fulfillment.pickup_attempt_result
            (tenant_id, source_id, object_ref, outcome)
);

CREATE TABLE transport_fulfillment.disposition_acceptance (
    tenant_id           text        NOT NULL,
    item_ref            text        NOT NULL,

    basis               text        NOT NULL,
    kind                text        NOT NULL,
    accepted_objects    jsonb       NOT NULL DEFAULT '[]'::jsonb,
    decline_basis       text,
    movement_authority  text,
    decided_at          timestamptz NOT NULL,
    content_digest      text        NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT disposition_acceptance_pkey
        PRIMARY KEY (tenant_id, item_ref),

    CONSTRAINT disposition_acceptance_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(item_ref) <> ''
            AND btrim(basis) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT disposition_acceptance_kind_closed
        CHECK (kind IN ('ACCEPTED', 'PARTIALLY_ACCEPTED', 'DECLINED')),

    CONSTRAINT disposition_acceptance_objects_shaped
        CHECK (
            accepted_objects IS NOT NULL
            AND jsonb_typeof(accepted_objects) = 'array'
        ),

    CONSTRAINT disposition_acceptance_kind_shape
        CHECK (
            (kind = 'ACCEPTED'
                AND jsonb_array_length(accepted_objects) > 0
                AND decline_basis IS NULL
                AND movement_authority IS NOT NULL AND btrim(movement_authority) <> '')
            OR (kind = 'PARTIALLY_ACCEPTED'
                AND jsonb_array_length(accepted_objects) > 0
                AND decline_basis IS NOT NULL AND btrim(decline_basis) <> ''
                AND movement_authority IS NOT NULL AND btrim(movement_authority) <> '')
            OR (kind = 'DECLINED'
                AND jsonb_array_length(accepted_objects) = 0
                AND decline_basis IS NOT NULL AND btrim(decline_basis) <> ''
                AND movement_authority IS NULL)
        )
);
