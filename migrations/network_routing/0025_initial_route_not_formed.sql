-- 初始路由未形成登记（ADR-0273 决定二）：一件包裹的初始路由判断停在重投推不动的一格时，原因与对象落在这里，接受决定信封
-- 照常入账，同一交接里已判成的包裹照常提交。只收重投推不动的格；依赖读不回之类重投推得动的整封回滚，不留行。它不是判断
-- 结果：不进 initial_route，不挡同一判断键以后再判，判成了落 initial_route，这一行照留作当时为何没形成的出处。
--
-- 主键取完整判断键，与 initial_route 同一组维：这一行答的是「这件包裹此刻为什么还没有初始路由」，同一判断键再停一次按最新
-- 这一次改写（ON CONFLICT DO UPDATE），不是某一次投递为什么没判——那是复核未判登记按触发关联留先到那一行的理由，这里不同。
-- correlation_id 是最近那一次交接的关联，accepted_at 是委托接受的业务时间，settled_at 是最近一次入账的处理时钟，recorded_at
-- 是这一行头一次落库的时刻。
--
-- 并列时 candidates、tied、tied_costs 都在：全部候选及各自的选择或淘汰依据、并列的那几家与各自的候选成本事实，授权角色裁的
-- 时候要看得见在哪几家之间、按什么金额打平。候选证据不全时 candidates 与 gaps 在：缺什么、什么条件再判。其余各格这几列都空。

CREATE TABLE network_routing.initial_route_not_formed (
    tenant_id            text        NOT NULL,
    customer_account_id  text        NOT NULL,
    shipment_request_id  text        NOT NULL,
    acceptance_baseline  text        NOT NULL,
    declared_parcel_id   text        NOT NULL,
    service_purpose      text        NOT NULL,

    correlation_id       text        NOT NULL,
    reason               text        NOT NULL,
    candidates           jsonb,
    tied                 jsonb,
    tied_costs           jsonb,
    gaps                 jsonb,
    accepted_at          timestamptz NOT NULL,
    settled_at           timestamptz NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT initial_route_not_formed_pkey
        PRIMARY KEY (tenant_id, customer_account_id, shipment_request_id,
                     acceptance_baseline, declared_parcel_id, service_purpose),

    CONSTRAINT initial_route_not_formed_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(correlation_id) <> ''
        ),

    -- 原因在库里也封闭（ports.InitialRouteNotFormedReason）。
    CONSTRAINT initial_route_not_formed_reason_closed
        CHECK (reason IN
            ('ROUTE_EVIDENCE_NOT_CONFIGURED', 'CANDIDATE_SPACE_NOT_ESTABLISHED', 'CANDIDATE_EVIDENCE_INCOMPLETE',
             'RANKING_FORM_NOT_CONFIGURED', 'CANDIDATES_TIED', 'CANDIDATE_COSTS_PENDING',
             'CANDIDATE_COSTS_UNPRICEABLE', 'CANDIDATE_COST_CURRENCIES_DIFFER', 'COST_SOURCE_NOT_CONFIGURED')),

    -- 在场件逐原因钉住：并列必带候选、并列的那几家与它们的成本；候选证据不全必带候选与缺口；其余各格一样都不带。
    CONSTRAINT initial_route_not_formed_parts_by_reason
        CHECK (
            (reason = 'CANDIDATES_TIED'
                AND candidates IS NOT NULL AND tied IS NOT NULL AND tied_costs IS NOT NULL AND gaps IS NULL)
            OR (reason = 'CANDIDATE_EVIDENCE_INCOMPLETE'
                AND candidates IS NOT NULL AND gaps IS NOT NULL AND tied IS NULL AND tied_costs IS NULL)
            OR (reason NOT IN ('CANDIDATES_TIED', 'CANDIDATE_EVIDENCE_INCOMPLETE')
                AND candidates IS NULL AND tied IS NULL AND tied_costs IS NULL AND gaps IS NULL)
        )
);
