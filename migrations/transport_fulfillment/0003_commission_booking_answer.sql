-- 运输委托、订舱申请与承运应答。
--
-- transport_commission 主键取（租户+委托）：Save 登记，Replace 只写取消/开始两列，
-- 快照与成员在 INSERT 里冻结。开始与取消互斥——已开始不能取消，已取消不能开始。
--
-- booking_request 主键取（租户+订舱）：申请幂等，同键第二份译`已有记录`。
--
-- booking_answer 主键取（租户+订舱）：一订舱一应答。已接受的行不会被拒绝覆盖——
-- 适配器只 INSERT ON CONFLICT DO NOTHING；改约走撤回与新订舱，不改这一行。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取长度。

CREATE TABLE transport_fulfillment.transport_commission (
    tenant_id        text        NOT NULL,
    commission_id    text        NOT NULL,

    provider_ref     text        NOT NULL,
    agreement_ref    text        NOT NULL,
    conditions_ref   text        NOT NULL,
    role_ref         text        NOT NULL,
    responsibility_ref text     NOT NULL,
    members          jsonb       NOT NULL,
    submitted_at     timestamptz NOT NULL,
    started_at       timestamptz,
    started_basis    text,
    cancelled_at     timestamptz,
    content_digest   text        NOT NULL,
    recorded_at      timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT transport_commission_pkey
        PRIMARY KEY (tenant_id, commission_id),

    CONSTRAINT transport_commission_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(commission_id) <> ''
            AND btrim(provider_ref) <> ''
            AND btrim(agreement_ref) <> ''
            AND btrim(conditions_ref) <> ''
            AND btrim(role_ref) <> ''
            AND btrim(responsibility_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT transport_commission_members_shaped
        CHECK (
            members IS NOT NULL
            AND jsonb_typeof(members) = 'array'
            AND jsonb_array_length(members) > 0
        ),

    -- 开始两半同在或同缺；等号碰上 NULL 给 NULL，必须先 IS NULL。
    CONSTRAINT transport_commission_started_coupled
        CHECK (
            (started_at IS NULL AND started_basis IS NULL)
            OR (started_at IS NOT NULL
                AND started_basis IS NOT NULL AND btrim(started_basis) <> ''
                AND started_at >= submitted_at)
        ),

    CONSTRAINT transport_commission_cancelled_not_before
        CHECK (cancelled_at IS NULL OR cancelled_at >= submitted_at),

    CONSTRAINT transport_commission_start_or_cancel
        CHECK (started_at IS NULL OR cancelled_at IS NULL)
);

CREATE TABLE transport_fulfillment.booking_request (
    tenant_id      text        NOT NULL,
    booking_id     text        NOT NULL,

    commission_id  text        NOT NULL,
    quantity       bigint      NOT NULL,
    unit_ref       text        NOT NULL,
    requested_at   timestamptz NOT NULL,
    cancelled_at   timestamptz,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT booking_request_pkey
        PRIMARY KEY (tenant_id, booking_id),

    CONSTRAINT booking_request_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(booking_id) <> ''
            AND btrim(commission_id) <> ''
            AND btrim(unit_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT booking_request_quantity_positive
        CHECK (quantity > 0),

    CONSTRAINT booking_request_cancelled_not_before
        CHECK (cancelled_at IS NULL OR cancelled_at >= requested_at)
);

CREATE TABLE transport_fulfillment.booking_answer (
    tenant_id      text        NOT NULL,
    booking_id     text        NOT NULL,

    acceptance_id  text        NOT NULL,
    outcome        text        NOT NULL,
    quantity       bigint      NOT NULL,
    basis          text,
    decided_at     timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT booking_answer_pkey
        PRIMARY KEY (tenant_id, booking_id),

    CONSTRAINT booking_answer_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(booking_id) <> ''
            AND btrim(acceptance_id) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT booking_answer_outcome_closed
        CHECK (outcome IN ('ACCEPTED', 'REFUSED', 'EXPIRED', 'WITHDRAWN')),

    CONSTRAINT booking_answer_outcome_shape
        CHECK (
            (outcome = 'ACCEPTED'
                AND quantity > 0
                AND basis IS NULL)
            OR (outcome IN ('REFUSED', 'EXPIRED', 'WITHDRAWN')
                AND quantity = 0
                AND basis IS NOT NULL AND btrim(basis) <> '')
        )
);
