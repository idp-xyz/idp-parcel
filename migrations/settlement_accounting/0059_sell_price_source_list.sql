-- 销售价格来源列表（ADR-0380 决定二、三，票 commercial-four-module-requirements/19）。
--
-- 合同带销售价格约定时，SELL 评价请求带合同声明的有序列表，不读采用册上的范围。列表不进请求的自然键：
-- 自然键仍是（租户、包裹、计费时点、代次）。pricing_scope_id 仍是列表的第一项，只带一个范围的旧行没有子行，
-- 读回仍是一个范围。
--
-- 两本采用册的计价范围改为可缺。已登的行不改。旧合同遇到没登范围的行，由发起路另答，不在这里补默认。

ALTER TABLE settlement_accounting.sell_billing_moment_adoption
    ALTER COLUMN pricing_scope_id DROP NOT NULL;

ALTER TABLE settlement_accounting.sell_billing_moment_adoption
    DROP CONSTRAINT sell_billing_moment_adoption_text_present;

ALTER TABLE settlement_accounting.sell_billing_moment_adoption
    ADD CONSTRAINT sell_billing_moment_adoption_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND (pricing_scope_id IS NULL OR btrim(pricing_scope_id) <> '')
        );

ALTER TABLE settlement_accounting.pre_acceptance_estimate_adoption
    ALTER COLUMN pricing_scope_id DROP NOT NULL;

ALTER TABLE settlement_accounting.pre_acceptance_estimate_adoption
    DROP CONSTRAINT pre_acceptance_estimate_adoption_text_present;

ALTER TABLE settlement_accounting.pre_acceptance_estimate_adoption
    ADD CONSTRAINT pre_acceptance_estimate_adoption_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND (pricing_scope_id IS NULL OR btrim(pricing_scope_id) <> '')
        );

ALTER TABLE settlement_accounting.sell_evaluation_initiation_stall
    DROP CONSTRAINT sell_evaluation_initiation_stall_account_only_when_reached;

ALTER TABLE settlement_accounting.sell_evaluation_initiation_stall
    ADD CONSTRAINT sell_evaluation_initiation_stall_account_only_when_reached
        CHECK (
            (stop_cause IN ('SELL_BILLING_MOMENT_NOT_ADOPTED', 'SELL_BILLING_MOMENT_SCOPE_ABSENT'))
            = (settlement_account_id IS NOT NULL)
        );

ALTER TABLE settlement_accounting.sell_evaluation_initiation_stall
    DROP CONSTRAINT sell_evaluation_initiation_stall_cause_closed;

ALTER TABLE settlement_accounting.sell_evaluation_initiation_stall
    ADD CONSTRAINT sell_evaluation_initiation_stall_cause_closed
        CHECK (stop_cause IN (
            'SETTLEMENT_POLICY_NOT_ADOPTED', 'SETTLEMENT_ACCOUNT_NOT_REGISTERED', 'SELL_BILLING_MOMENT_NOT_ADOPTED',
            'SELL_BILLING_MOMENT_SCOPE_ABSENT', 'PARCEL_HAS_NO_COMMERCIAL_RESOLUTION'
        ));

CREATE TABLE settlement_accounting.sell_evaluation_request_pricing_scope (
    tenant_id        text    NOT NULL,
    request_id       text    NOT NULL,
    ordinal          integer NOT NULL,
    pricing_scope_id text    NOT NULL,

    CONSTRAINT sell_evaluation_request_pricing_scope_pkey
        PRIMARY KEY (tenant_id, request_id, ordinal),

    CONSTRAINT sell_evaluation_request_pricing_scope_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES settlement_accounting.sell_evaluation_request (tenant_id, request_id),

    CONSTRAINT sell_evaluation_request_pricing_scope_ordinal_positive
        CHECK (ordinal >= 1),

    CONSTRAINT sell_evaluation_request_pricing_scope_text_present
        CHECK (btrim(pricing_scope_id) <> '')
);

-- 请求停下时，除了答出来的那一格，另逐个记下问到过的每一个来源各自的那一格。
-- 续办按命中的来源自己的停因核，不按请求的停因核；没问到的来源没有行，它上面的登记不续办。
-- 只带一个范围的请求没有这些行，续办仍只看请求自己的停因和 pricing_scope_id。

CREATE TABLE settlement_accounting.sell_evaluation_request_source_stop (
    tenant_id        text    NOT NULL,
    request_id       text    NOT NULL,
    ordinal          integer NOT NULL,
    pricing_scope_id text    NOT NULL,
    stop_cause       text    NOT NULL,

    CONSTRAINT sell_evaluation_request_source_stop_pkey
        PRIMARY KEY (tenant_id, request_id, ordinal),

    CONSTRAINT sell_evaluation_request_source_stop_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES settlement_accounting.sell_evaluation_request (tenant_id, request_id),

    CONSTRAINT sell_evaluation_request_source_stop_ordinal_positive
        CHECK (ordinal >= 1),

    CONSTRAINT sell_evaluation_request_source_stop_text_present
        CHECK (btrim(pricing_scope_id) <> '' AND btrim(stop_cause) <> '')
);
