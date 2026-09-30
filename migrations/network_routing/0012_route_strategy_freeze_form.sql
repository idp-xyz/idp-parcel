-- 路由策略版本声明的冻结边界形态与剩余段数取值。两格都空即未声明，不是未冻结。

ALTER TABLE network_routing.route_strategy_version
    ADD COLUMN freeze_form text,
    ADD COLUMN freeze_remaining_segments integer;

ALTER TABLE network_routing.route_strategy_version
    ADD CONSTRAINT route_strategy_version_freeze_form_known
        CHECK (freeze_form IS NULL OR freeze_form IN ('REMAINING_SEGMENT_COUNT')),
    ADD CONSTRAINT route_strategy_version_freeze_limit
        CHECK (freeze_remaining_segments IS NULL OR freeze_remaining_segments >= 0),
    ADD CONSTRAINT route_strategy_version_freeze_pair
        CHECK (
            (freeze_form IS NULL AND freeze_remaining_segments IS NULL)
            OR (freeze_form IS NOT NULL AND freeze_remaining_segments IS NOT NULL)
        );
