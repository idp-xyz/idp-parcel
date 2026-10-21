-- 评价请求两本册加代次与停因（ADR-0229 决定一、三）。
--
-- 代次：同一自然键上的请求排成一条链。首代没有前一代；续办出来的各代指名前一代并带续办依据（一次参数登记或一次运营
-- 续办）。自然键的唯一约束因此换成（自然键、代次）：自然键仍守「同一个问题只问一次」，代次让补登之后能再问一次而
-- 不改写前一代。一份请求至多一个下一代，由前一代上的唯一约束守——同一封「参数已登记」重投、两路续办并发，库上只落
-- 一份下一代。前一代经外键指回同一本册。
--
-- 停因：请求停下时记停在哪一格，续办按它取等着的请求。BUY 册没有状态格：形成了的由派生的预期成本版本可查，停下的
-- 记停因。SELL 册的状态格是停因的粗分，两列成对受 CHECK 约束；本迁移给它加`未形成`一格（计价没形成评价、评价失败）。
--
-- 迁移前已记成`不可计价`、`冲突`的 SELL 请求，停因由状态唯一确定，照填；记成`待判断`的当时没记是哪一种，填
-- PENDING_JUDGMENT_CAUSE_UNRECORDED，不猜。停下时刻同样没记，取请求的登记时刻，只作队列定序的下界。迁移前停在
-- `已提交`而实际已卡住的请求（计价没形成评价、评价失败）从行上看不出来，不在这里补。

ALTER TABLE settlement_accounting.evaluation_request
    ADD COLUMN generation          integer NOT NULL DEFAULT 1,
    ADD COLUMN previous_request_id text,
    ADD COLUMN resume_basis        text,
    ADD COLUMN stop_cause          text,
    ADD COLUMN stopped_at          timestamptz;

ALTER TABLE settlement_accounting.evaluation_request
    ALTER COLUMN generation DROP DEFAULT;

ALTER TABLE settlement_accounting.evaluation_request
    DROP CONSTRAINT evaluation_request_natural_key_unique;

ALTER TABLE settlement_accounting.evaluation_request
    ADD CONSTRAINT evaluation_request_natural_key_generation_unique
        UNIQUE (tenant_id, scope_ref, purpose, source_digest, generation),
    ADD CONSTRAINT evaluation_request_previous_unique
        UNIQUE (tenant_id, previous_request_id),
    ADD CONSTRAINT evaluation_request_previous_fkey
        FOREIGN KEY (tenant_id, previous_request_id)
        REFERENCES settlement_accounting.evaluation_request (tenant_id, request_id),
    ADD CONSTRAINT evaluation_request_lineage
        CHECK (
            generation >= 1
            AND (generation = 1) = (previous_request_id IS NULL)
            AND (generation = 1 OR resume_basis IS NOT NULL)
            AND (previous_request_id IS NULL OR btrim(previous_request_id) <> '')
            AND (resume_basis IS NULL OR btrim(resume_basis) <> '')
        ),
    ADD CONSTRAINT evaluation_request_stop_paired
        CHECK ((stop_cause IS NULL) = (stopped_at IS NULL)),
    ADD CONSTRAINT evaluation_request_stop_cause_closed
        CHECK (stop_cause IS NULL OR stop_cause IN (
            'PRICE_CARD_NOT_CONFIGURED', 'PRICE_CARD_APPLICABILITY_CONFLICT', 'PRICING_INPUT_UNAVAILABLE',
            'EVALUATION_PENDING_JUDGMENT', 'EVALUATION_UNRATABLE', 'EVALUATION_CONFLICT', 'EVALUATION_FAILED',
            'AMOUNT_ROUNDING_UNDECLARED', 'CONVERSION_STEP_MISSING', 'EVALUATION_CATALOGUE_UNRESOLVED',
            'EVALUATION_SERIES_UNRESOLVED', 'EVALUATION_RATE_NOT_FOUND', 'EVALUATION_INPUT_INCOMPLETE'
        ));

CREATE INDEX evaluation_request_stopped_idx
    ON settlement_accounting.evaluation_request (tenant_id, stopped_at, request_id)
    WHERE stop_cause IS NOT NULL;

ALTER TABLE settlement_accounting.sell_evaluation_request
    ADD COLUMN generation          integer NOT NULL DEFAULT 1,
    ADD COLUMN previous_request_id text,
    ADD COLUMN resume_basis        text,
    ADD COLUMN stop_cause          text,
    ADD COLUMN stopped_at          timestamptz;

ALTER TABLE settlement_accounting.sell_evaluation_request
    ALTER COLUMN generation DROP DEFAULT;

UPDATE settlement_accounting.sell_evaluation_request
   SET stop_cause = CASE state
           WHEN 'UNRATABLE' THEN 'EVALUATION_UNRATABLE'
           WHEN 'CONFLICT' THEN 'EVALUATION_CONFLICT'
           WHEN 'PENDING_JUDGMENT' THEN 'PENDING_JUDGMENT_CAUSE_UNRECORDED'
       END,
       stopped_at = recorded_at
 WHERE state IN ('UNRATABLE', 'CONFLICT', 'PENDING_JUDGMENT');

ALTER TABLE settlement_accounting.sell_evaluation_request
    DROP CONSTRAINT sell_evaluation_request_natural_key,
    DROP CONSTRAINT sell_evaluation_request_state_closed;

ALTER TABLE settlement_accounting.sell_evaluation_request
    ADD CONSTRAINT sell_evaluation_request_natural_key_generation
        UNIQUE (tenant_id, parcel_id, billing_moment, generation),
    ADD CONSTRAINT sell_evaluation_request_previous_unique
        UNIQUE (tenant_id, previous_request_id),
    ADD CONSTRAINT sell_evaluation_request_previous_fkey
        FOREIGN KEY (tenant_id, previous_request_id)
        REFERENCES settlement_accounting.sell_evaluation_request (tenant_id, request_id),
    ADD CONSTRAINT sell_evaluation_request_lineage
        CHECK (
            generation >= 1
            AND (generation = 1) = (previous_request_id IS NULL)
            AND (generation = 1 OR resume_basis IS NOT NULL)
            AND (previous_request_id IS NULL OR btrim(previous_request_id) <> '')
            AND (resume_basis IS NULL OR btrim(resume_basis) <> '')
        ),
    ADD CONSTRAINT sell_evaluation_request_state_closed
        CHECK (state IN ('SUBMITTED', 'FORMED', 'PENDING_JUDGMENT', 'UNRATABLE', 'CONFLICT', 'NOT_FORMED')),
    ADD CONSTRAINT sell_evaluation_request_stop_paired
        CHECK ((stop_cause IS NULL) = (stopped_at IS NULL)),
    -- COALESCE 让「停下却没有停因」落成假而不是 NULL：CHECK 遇 NULL 放行，裸写 stop_cause IN (...) 会把它放过去。
    ADD CONSTRAINT sell_evaluation_request_stop_cause_matches_state
        CHECK (
            (state IN ('SUBMITTED', 'FORMED') AND stop_cause IS NULL)
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

CREATE INDEX sell_evaluation_request_stopped_idx
    ON settlement_accounting.sell_evaluation_request (tenant_id, stopped_at, request_id)
    WHERE stop_cause IS NOT NULL;
