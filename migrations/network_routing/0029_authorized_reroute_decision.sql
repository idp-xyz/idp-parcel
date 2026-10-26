-- 授权改路决定册（ADR-0284 决定五）：授权角色据一份改路建议择一候选形成的改路决定与新计划。
--
-- 主键取（租户，建议所在复核的触发关联）：一份建议至多一份决定，两个授权角色同时决定时撞键即`已有记录`（ON CONFLICT 代数，
-- ADR-0031），不覆盖先到者。外键指回复核册那一行：没有建议的决定是凭空的人工改路。判断键各维平铺，复核取判断起点与段窗口
-- 读口按它们并读这一册的新计划；新计划本体与复核册的 new_plan 同一份行模型。
--
-- 决定方是信封里认证出的操作者（`OPERATOR/<发行方>#<sub>`），依据是决定方给的引用；决定时刻就是新计划生效的时刻（决定四）。
-- 本迁移不种任何行。

CREATE TABLE network_routing.authorized_reroute_decision (
    tenant_id                 text        NOT NULL,
    suggestion_correlation_id text        NOT NULL,

    customer_account_id       text        NOT NULL,
    shipment_request_id       text        NOT NULL,
    acceptance_baseline       text        NOT NULL,
    declared_parcel_id        text        NOT NULL,
    service_purpose           text        NOT NULL,

    chosen_candidate          text        NOT NULL,
    original_plan             text        NOT NULL,
    new_plan_version          text        NOT NULL,
    new_plan                  jsonb       NOT NULL,
    decided_by                text        NOT NULL,
    decision_basis            text        NOT NULL,
    decided_at                timestamptz NOT NULL,
    recorded_at               timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT authorized_reroute_decision_pkey
        PRIMARY KEY (tenant_id, suggestion_correlation_id),

    CONSTRAINT authorized_reroute_decision_suggestion_fk
        FOREIGN KEY (tenant_id, suggestion_correlation_id)
        REFERENCES network_routing.route_reassessment (tenant_id, correlation_id),

    CONSTRAINT authorized_reroute_decision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(suggestion_correlation_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(shipment_request_id) <> ''
            AND btrim(acceptance_baseline) <> ''
            AND btrim(declared_parcel_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(chosen_candidate) <> ''
            AND btrim(original_plan) <> ''
            AND btrim(new_plan_version) <> ''
            AND btrim(decided_by) <> ''
            AND btrim(decision_basis) <> ''
        ),

    -- 替代关系：新计划换了版本，平铺的版本就是本体里那一版——段窗口读口按平铺列定位，两处说的不是同一版就是坏数据。
    CONSTRAINT authorized_reroute_decision_replaces_another_plan
        CHECK (new_plan_version <> original_plan),
    CONSTRAINT authorized_reroute_decision_plan_version_matches
        CHECK (new_plan ->> 'version' = new_plan_version)
);

CREATE INDEX authorized_reroute_decision_by_judgment_key
    ON network_routing.authorized_reroute_decision
       (tenant_id, customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose);

CREATE INDEX authorized_reroute_decision_by_plan_version
    ON network_routing.authorized_reroute_decision (tenant_id, new_plan_version);
