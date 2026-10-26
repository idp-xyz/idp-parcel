-- 两本评价请求册加「负向结束（零额）」（ADR-0306 决定二、三）：评价已完成而合计为零是这张卡给出的价格，不是两侧
-- 矛盾，也不是停下。请求记下这一格、入账不重投、不建费用行或成本行；它不带停因，续办队列按停因取件，自然取不到它。
--
-- SELL 请求册的状态格加 ZERO_TOTAL 一格：不带费用标识（只有 FORMED 带，原约束照旧），不带停因。
-- BUY 请求册没有状态格，加一列 negative_end，只收 ZERO_TOTAL 一个词，与停因互斥——一份请求只有一种去向。
-- 存量行没有这一格：迁移前合计为零的评价按矛盾上抛、请求停在`已提交`，从行上看不出来，不在这里回补。

ALTER TABLE settlement_accounting.sell_evaluation_request
    DROP CONSTRAINT sell_evaluation_request_state_closed,
    DROP CONSTRAINT sell_evaluation_request_stop_cause_matches_state;

ALTER TABLE settlement_accounting.sell_evaluation_request
    ADD CONSTRAINT sell_evaluation_request_state_closed
        CHECK (state IN ('SUBMITTED', 'FORMED', 'PENDING_JUDGMENT', 'UNRATABLE', 'CONFLICT', 'NOT_FORMED', 'ZERO_TOTAL')),
    -- COALESCE 让「停下却没有停因」落成假而不是 NULL：CHECK 遇 NULL 放行，裸写 stop_cause IN (...) 会把它放过去。
    ADD CONSTRAINT sell_evaluation_request_stop_cause_matches_state
        CHECK (
            (state IN ('SUBMITTED', 'FORMED', 'ZERO_TOTAL') AND stop_cause IS NULL)
            OR (state = 'PENDING_JUDGMENT' AND COALESCE(stop_cause, '') IN (
                'EVALUATION_PENDING_JUDGMENT', 'AMOUNT_ROUNDING_UNDECLARED', 'FEE_ITEM_MAPPING_INCOMPLETE',
                'EVALUATION_CATALOGUE_UNRESOLVED', 'EVALUATION_SERIES_UNRESOLVED', 'EVALUATION_RATE_NOT_FOUND',
                'EVALUATION_INPUT_INCOMPLETE', 'PENDING_JUDGMENT_CAUSE_UNRECORDED'))
            OR (state = 'UNRATABLE' AND COALESCE(stop_cause, '') = 'EVALUATION_UNRATABLE')
            OR (state = 'CONFLICT' AND COALESCE(stop_cause, '') = 'EVALUATION_CONFLICT')
            OR (state = 'NOT_FORMED' AND COALESCE(stop_cause, '') IN (
                'EVALUATION_FAILED', 'PRICE_CARD_NOT_CONFIGURED', 'PRICE_CARD_APPLICABILITY_CONFLICT',
                'PRICING_INPUT_UNAVAILABLE'))
        );

ALTER TABLE settlement_accounting.evaluation_request
    ADD COLUMN negative_end text;

ALTER TABLE settlement_accounting.evaluation_request
    ADD CONSTRAINT evaluation_request_negative_end_closed
        CHECK (negative_end IS NULL OR negative_end = 'ZERO_TOTAL'),
    ADD CONSTRAINT evaluation_request_negative_end_excludes_stop
        CHECK (negative_end IS NULL OR stop_cause IS NULL);
