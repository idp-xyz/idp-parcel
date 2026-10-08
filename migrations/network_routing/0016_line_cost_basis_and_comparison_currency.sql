-- 票 routing-first-cut/10（ADR-0148 决定四）：候选成本的两类目录内容。
--
-- 一、路由策略版本加比较币种与所引价格政策：都是租户取值，未登记即缺省（NULL），
-- 不写死任何默认币种。比较币种没登记时，取数侧只在各段同币种时合成比较。
-- 二、线路版本各段（segment_index 对段链数组下标，序即语义）的成本依据引用：
-- 外包段引 BUY 价卡（SUPPLIER_BUY_PLAN），自营段引内部价格政策版本（INTERNAL_POLICY）。
-- 引用的正文不在这里：方案正文归 parcel-pricing，政策正文归 party-commercial，本表只持引用。

ALTER TABLE network_routing.route_strategy_version
    ADD COLUMN comparison_currency text,
    ADD COLUMN comparison_price_policy text;

ALTER TABLE network_routing.route_strategy_version
    ADD CONSTRAINT route_strategy_version_comparison_currency_not_blank
        CHECK (comparison_currency IS NULL OR btrim(comparison_currency) <> ''),
    ADD CONSTRAINT route_strategy_version_comparison_policy_not_blank
        CHECK (comparison_price_policy IS NULL OR btrim(comparison_price_policy) <> '');

CREATE TABLE network_routing.line_cost_basis (
    tenant_id     text    NOT NULL,
    line_code     text    NOT NULL,
    version       integer NOT NULL,
    segment_index integer NOT NULL,

    basis_kind    text    NOT NULL,
    basis_ref     text    NOT NULL,

    CONSTRAINT line_cost_basis_pkey
        PRIMARY KEY (tenant_id, line_code, version, segment_index),
    CONSTRAINT line_cost_basis_index_nonnegative
        CHECK (segment_index >= 0),
    CONSTRAINT line_cost_basis_kind_known
        CHECK (basis_kind IN ('SUPPLIER_BUY_PLAN', 'INTERNAL_POLICY')),
    CONSTRAINT line_cost_basis_ref_not_blank
        CHECK (btrim(basis_ref) <> ''),
    CONSTRAINT line_cost_basis_known_line
        FOREIGN KEY (tenant_id, line_code, version)
        REFERENCES network_routing.line_version (tenant_id, line_code, version)
);