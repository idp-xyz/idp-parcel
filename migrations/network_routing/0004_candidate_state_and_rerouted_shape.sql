-- 前向 DROP+ADD：PostgreSQL 不能改 CHECK 表达式，只能丢掉再钉。
--
-- 0003 只前向了 reroute_coupling。已施加 0002 原文的库仍缺另外两条新表达式：
-- candidate_state_closed（NULL IN 会按 NULL 放行）与 rerouted_shape（可空列等号
-- 碰上 NULL 给 NULL）。表达式与当前 0002 文本一致。不要改写 0002/0003。

ALTER TABLE network_routing.route_reassessment
    DROP CONSTRAINT route_reassessment_candidate_state_closed;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_candidate_state_closed
    CHECK (
        (conclusion = 'STILL_APPLICABLE' AND candidate_state IS NULL)
        OR (conclusion <> 'STILL_APPLICABLE'
            AND candidate_state IS NOT NULL
            AND candidate_state IN
            ('CANDIDATES_AVAILABLE', 'NO_QUALIFIED_CANDIDATES', 'CANDIDATE_REVIEW_UNDECIDED'))
    );

ALTER TABLE network_routing.route_reassessment
    DROP CONSTRAINT route_reassessment_rerouted_shape;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_rerouted_shape
    CHECK (conclusion <> 'REROUTED' OR (
        reviewed_plan IS NOT NULL AND lapse_basis IS NOT NULL
        AND new_plan IS NOT NULL AND decision IS NOT NULL
        AND suggestion IS NULL
        AND reroute_state IS NOT DISTINCT FROM 'AUTOMATIC_ALLOWED'));
