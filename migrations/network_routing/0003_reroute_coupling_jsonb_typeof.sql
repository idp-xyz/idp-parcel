-- 前向 DROP+ADD：PostgreSQL 不能改 CHECK 表达式，只能丢掉再钉。
--
-- route_reassessment_reroute_coupling 里 jsonb_array_length 之前补 jsonb_typeof =
-- 'array'。IS NOT NULL 挡住了 NULL 三值缝；对象/标量仍可能让 length 报错或按 NULL
-- 放行（f822e1d 入册的纪律）。

ALTER TABLE network_routing.route_reassessment
    DROP CONSTRAINT route_reassessment_reroute_coupling;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_reroute_coupling
    CHECK (
        (reroute_state IS NULL AND reroute_blockers IS NULL AND suggestion IS NULL)
        OR (reroute_state = 'AUTOMATIC_ALLOWED'
            AND reroute_blockers IS NULL AND suggestion IS NULL)
        OR (reroute_state = 'SUGGESTION_ONLY'
            AND reroute_blockers IS NOT NULL
            AND jsonb_typeof(reroute_blockers) = 'array'
            AND jsonb_array_length(reroute_blockers) > 0)
        OR (reroute_state = 'BARRED'
            AND reroute_blockers IS NOT NULL
            AND jsonb_typeof(reroute_blockers) = 'array'
            AND jsonb_array_length(reroute_blockers) > 0
            AND suggestion IS NULL)
    );
