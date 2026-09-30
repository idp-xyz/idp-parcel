-- 路由策略版本声明的自动改路形态与成本改善阈值。两格都空即未声明，不是允许，也不是不允许。

ALTER TABLE network_routing.route_strategy_version
    ADD COLUMN auto_reroute_form text,
    ADD COLUMN auto_reroute_improvement_threshold_minor integer;

ALTER TABLE network_routing.route_strategy_version
    ADD CONSTRAINT route_strategy_version_auto_reroute_form_known
        CHECK (auto_reroute_form IS NULL OR auto_reroute_form IN ('COST_IMPROVEMENT')),
    ADD CONSTRAINT route_strategy_version_auto_reroute_threshold
        CHECK (auto_reroute_improvement_threshold_minor IS NULL OR auto_reroute_improvement_threshold_minor >= 0),
    ADD CONSTRAINT route_strategy_version_auto_reroute_pair
        CHECK (
            (auto_reroute_form IS NULL AND auto_reroute_improvement_threshold_minor IS NULL)
            OR (auto_reroute_form IS NOT NULL AND auto_reroute_improvement_threshold_minor IS NOT NULL)
        );
