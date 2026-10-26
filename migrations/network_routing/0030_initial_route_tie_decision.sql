-- 最低成本并列的授权裁决证据（ADR-0309 决定四）：授权角色在并列的那几家之间择一，形成这件包裹的首个当前有效计划。计划本体
-- 落 initial_route（与自动形成的计划同一册、同一键），这一册只记「谁凭什么把并列裁给了谁」：所择候选、决定依据、决定方与
-- 决定时刻。没有这一行的 initial_route 计划是自动形成的。
--
-- 主键取完整判断键，与 initial_route 同一组维：一件包裹至多一次裁决，外键指回那一行，先有计划后有证据；外键同时指未形成
-- 登记那一行，没有并列登记的裁决是凭空的人工选路。登记行之后会被同键的又一次交接改写，所以不钉它当前的原因，只钉行在。
-- 决定方是信封里认证出的操作者（`OPERATOR/<发行方>#<sub>`），依据是决定方给的引用；决定时刻就是计划生效的时刻。
-- 本迁移不种任何行。

CREATE TABLE network_routing.initial_route_tie_decision (
    tenant_id            text        NOT NULL,
    customer_account_id  text        NOT NULL,
    shipment_request_id  text        NOT NULL,
    acceptance_baseline  text        NOT NULL,
    declared_parcel_id   text        NOT NULL,
    service_purpose      text        NOT NULL,

    chosen_candidate     text        NOT NULL,
    plan_version         text        NOT NULL,
    decided_by           text        NOT NULL,
    decision_basis       text        NOT NULL,
    decided_at           timestamptz NOT NULL,
    recorded_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT initial_route_tie_decision_pkey
        PRIMARY KEY (tenant_id, customer_account_id, shipment_request_id,
                     acceptance_baseline, declared_parcel_id, service_purpose),

    CONSTRAINT initial_route_tie_decision_route_fk
        FOREIGN KEY (tenant_id, customer_account_id, shipment_request_id,
                     acceptance_baseline, declared_parcel_id, service_purpose)
        REFERENCES network_routing.initial_route
                   (tenant_id, customer_account_id, shipment_request_id,
                    acceptance_baseline, declared_parcel_id, service_purpose),

    CONSTRAINT initial_route_tie_decision_registration_fk
        FOREIGN KEY (tenant_id, customer_account_id, shipment_request_id,
                     acceptance_baseline, declared_parcel_id, service_purpose)
        REFERENCES network_routing.initial_route_not_formed
                   (tenant_id, customer_account_id, shipment_request_id,
                    acceptance_baseline, declared_parcel_id, service_purpose),

    CONSTRAINT initial_route_tie_decision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(chosen_candidate) <> ''
            AND btrim(plan_version) <> ''
            AND btrim(decided_by) <> ''
            AND btrim(decision_basis) <> ''
        )
);

CREATE INDEX initial_route_tie_decision_by_plan_version
    ON network_routing.initial_route_tie_decision (tenant_id, plan_version);
