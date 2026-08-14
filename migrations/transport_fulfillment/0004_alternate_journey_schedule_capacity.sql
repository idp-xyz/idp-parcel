-- 替代/退运旅程、班次与容量池。
--
-- alternate_journey 主键取（租户+原旅程+目的+处置依据）：同一处置决定不开两条替代
-- 旅程。新身份与原旅程必须不同；成员非空数组。
--
-- transport_schedule 主键取（租户+班次）：同一班次标识只建一次。班次行不承载容量
-- 或订舱——那两件事各自成表。
--
-- capacity_pool 主键取（租户+池）：Save 建空池，Replace 只换预占子表与记录时刻，
-- 不改班次/单位/容量。预占三量守恒钉在子表单行 CHECK 上；池级可用量是领域按时点
-- 算的，库不存。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取长度。

CREATE TABLE transport_fulfillment.alternate_journey (
    tenant_id          text        NOT NULL,
    original_journey   text        NOT NULL,
    purpose            text        NOT NULL,
    basis              text        NOT NULL,

    journey_id         text        NOT NULL,
    basis_kind         text        NOT NULL,
    members            jsonb       NOT NULL,
    started_at         timestamptz NOT NULL,
    content_digest     text        NOT NULL,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT alternate_journey_pkey
        PRIMARY KEY (tenant_id, original_journey, purpose, basis),

    CONSTRAINT alternate_journey_identity_unique
        UNIQUE (tenant_id, journey_id),

    CONSTRAINT alternate_journey_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(original_journey) <> ''
            AND btrim(purpose) <> ''
            AND btrim(basis) <> ''
            AND btrim(journey_id) <> ''
            AND btrim(basis_kind) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT alternate_journey_purpose_closed
        CHECK (purpose IN ('ALTERNATE', 'RETURN')),

    CONSTRAINT alternate_journey_basis_kind_closed
        CHECK (basis_kind IN ('SERVICE_DISPOSITION', 'REGULATORY_DISPOSITION')),

    CONSTRAINT alternate_journey_identity_independent
        CHECK (journey_id <> original_journey),

    CONSTRAINT alternate_journey_members_shaped
        CHECK (
            members IS NOT NULL
            AND jsonb_typeof(members) = 'array'
            AND jsonb_array_length(members) > 0
        )
);

CREATE TABLE transport_fulfillment.transport_schedule (
    tenant_id      text        NOT NULL,
    schedule_id    text        NOT NULL,

    direction      text        NOT NULL,
    departs_at     timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT transport_schedule_pkey
        PRIMARY KEY (tenant_id, schedule_id),

    CONSTRAINT transport_schedule_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(schedule_id) <> ''
            AND btrim(direction) <> ''
            AND btrim(content_digest) <> ''
        )
);

CREATE TABLE transport_fulfillment.capacity_pool (
    tenant_id      text        NOT NULL,
    pool_id        text        NOT NULL,

    schedule_id    text        NOT NULL,
    unit_ref       text        NOT NULL,
    capacity       bigint      NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT capacity_pool_pkey
        PRIMARY KEY (tenant_id, pool_id),

    CONSTRAINT capacity_pool_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pool_id) <> ''
            AND btrim(schedule_id) <> ''
            AND btrim(unit_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT capacity_pool_capacity_positive
        CHECK (capacity > 0)
);

CREATE TABLE transport_fulfillment.capacity_reservation (
    tenant_id       text        NOT NULL,
    pool_id         text        NOT NULL,
    reservation_id  text        NOT NULL,

    quantity        bigint      NOT NULL,
    valid_until     timestamptz NOT NULL,
    released        bigint      NOT NULL,
    consumed        bigint      NOT NULL,

    CONSTRAINT capacity_reservation_pkey
        PRIMARY KEY (tenant_id, pool_id, reservation_id),

    CONSTRAINT capacity_reservation_pool_fkey
        FOREIGN KEY (tenant_id, pool_id)
        REFERENCES transport_fulfillment.capacity_pool (tenant_id, pool_id),

    CONSTRAINT capacity_reservation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pool_id) <> ''
            AND btrim(reservation_id) <> ''
        ),

    CONSTRAINT capacity_reservation_quantity_positive
        CHECK (quantity > 0),

    CONSTRAINT capacity_reservation_released_non_negative
        CHECK (released >= 0),

    CONSTRAINT capacity_reservation_consumed_non_negative
        CHECK (consumed >= 0),

    CONSTRAINT capacity_reservation_conserved
        CHECK (released + consumed <= quantity)
);
