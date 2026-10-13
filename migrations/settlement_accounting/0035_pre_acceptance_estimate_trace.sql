-- 受理前估价留痕（ADR-0179 决定六）：一次接受前控制的金额按哪一行采用、哪张价卡、哪几份试算评价连同它们的版本清单合出来。
--
-- 键与冻结、信用暴露两本账同形（租户 + 结算作用域 + 控制请求标识），写在同一事务里。不给两本账加列：`业务限制`从不入那两本账，
-- 而受限的那一次同样要答得出是拿哪个金额判的；组合策略两项共用一个金额，加列就要存两份。
-- 只增不改：同键再写同一份是重放，内容不同是冲突，都不动已落的行。逐件试算平铺在 trials 里，复算要的就是这一份。

CREATE TABLE settlement_accounting.pre_acceptance_estimate_trace (
    tenant_id              text        NOT NULL,
    legal_entity           text        NOT NULL,
    account_id             text        NOT NULL,
    currency               text        NOT NULL,
    control_request_id     text        NOT NULL,

    shipment_request_ref   text        NOT NULL,
    submission_version_ref text        NOT NULL,
    trial_at               timestamptz NOT NULL,
    form                   text        NOT NULL,
    pricing_scope_id       text        NOT NULL,
    price_card             text        NOT NULL,
    total_minor            bigint      NOT NULL,
    trials                 jsonb       NOT NULL,
    recorded_at            timestamptz NOT NULL,
    inserted_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pre_acceptance_estimate_trace_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency, control_request_id),

    CONSTRAINT pre_acceptance_estimate_trace_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(control_request_id) <> ''
            AND btrim(shipment_request_ref) <> ''
            AND btrim(submission_version_ref) <> ''
            AND btrim(pricing_scope_id) <> ''
            AND btrim(price_card) <> ''
        ),

    CONSTRAINT pre_acceptance_estimate_trace_form_closed
        CHECK (form IN ('PER_PARCEL_SELL_TRIAL_SUM')),

    -- 合计不是正数的估价没有形成，到不了这一册。
    CONSTRAINT pre_acceptance_estimate_trace_total_positive
        CHECK (total_minor > 0),

    CONSTRAINT pre_acceptance_estimate_trace_trials_listed
        CHECK (jsonb_typeof(trials) = 'array' AND jsonb_array_length(trials) > 0)
);
