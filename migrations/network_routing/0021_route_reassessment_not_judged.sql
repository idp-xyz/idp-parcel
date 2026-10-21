-- 未判触发登记（ADR-0235）：复核触发已由 route_handoff_log 保全，这一次却不形成判断时，原因落在这里。
-- 只收重投推不动的格；依赖读不回之类重投推得动的整笔回滚，不留行。它不是复核结论：不进
-- route_reassessment、不改路由历史，也不挡同一触发以后再判——携带同一关联的另一封信封再来时编排重新判
-- 一遍，判成了落 route_reassessment，这一行照留作当时为何没判的出处。
--
-- 主键取（租户，触发关联），同一触发只留先到那一行（ON CONFLICT DO NOTHING）。判断键平铺成列，
-- 日后按包裹或按原因列出等待中的触发不必解析关联串。trigger_occurred_at 是触发的业务时间，
-- settled_at 是入账那一刻的处理时钟，recorded_at 是落库时刻。

CREATE TABLE network_routing.route_reassessment_not_judged (
    tenant_id            text        NOT NULL,
    correlation_id       text        NOT NULL,

    customer_account_id  text        NOT NULL,
    shipment_request_id  text        NOT NULL,
    acceptance_baseline  text        NOT NULL,
    declared_parcel_id   text        NOT NULL,
    service_purpose      text        NOT NULL,

    reason               text        NOT NULL,
    trigger_occurred_at  timestamptz NOT NULL,
    settled_at           timestamptz NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT route_reassessment_not_judged_pkey
        PRIMARY KEY (tenant_id, correlation_id),

    CONSTRAINT route_reassessment_not_judged_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(correlation_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
        ),

    -- 原因在库里也封闭（ports.NotJudgedReason）。
    CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED'))
);
