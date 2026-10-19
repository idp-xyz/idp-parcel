-- SELL 评价请求册（ADR-0190 决定一）：铸造标识是身份，自然键（租户、包裹、计费时点）只守幂等——同一件包裹在同一
-- 时点只发起一次，库上的唯一约束守它。与 BUY 的 evaluation_request 是两张表，BUY 那本一字不动。
-- 请求只记「问了什么」，没有金额、换算或价卡列；settlement_currency 是登记时从不可变的结算账户行快照下来的结算币种，
-- 计价建快照取它，不另读账户册。
-- state 登记时是 SUBMITTED，其余各格由形成那一半改记（ADR-0190 决定五）；只有 FORMED 带形成的预估客户费用标识，
-- 按包裹找当前有效的预估客户费用走这一对。不登任何租户的行。

CREATE TABLE settlement_accounting.sell_evaluation_request (
    tenant_id           text        NOT NULL,
    request_id          text        NOT NULL,
    parcel_id           text        NOT NULL,
    account_id          text        NOT NULL,
    settlement_currency text        NOT NULL,
    pricing_scope_id    text        NOT NULL,
    billing_moment      text        NOT NULL,
    source_fact_ref     text        NOT NULL,
    pricing_basis_at    timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL,
    state               text        NOT NULL,
    customer_charge_id  text,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sell_evaluation_request_pkey
        PRIMARY KEY (tenant_id, request_id),

    CONSTRAINT sell_evaluation_request_natural_key
        UNIQUE (tenant_id, parcel_id, billing_moment),

    CONSTRAINT sell_evaluation_request_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(request_id) <> ''
            AND btrim(parcel_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(settlement_currency) <> ''
            AND btrim(pricing_scope_id) <> ''
            AND btrim(source_fact_ref) <> ''
        ),

    CONSTRAINT sell_evaluation_request_moment_closed
        CHECK (billing_moment IN ('NETWORK_INTAKE_ADOPTED', 'EFFECTIVE_DELIVERY_FINALIZED')),

    CONSTRAINT sell_evaluation_request_state_closed
        CHECK (state IN ('SUBMITTED', 'FORMED', 'PENDING_JUDGMENT', 'UNRATABLE', 'CONFLICT')),

    CONSTRAINT sell_evaluation_request_charge_only_when_formed
        CHECK (
            (state = 'FORMED') = (customer_charge_id IS NOT NULL)
            AND (customer_charge_id IS NULL OR btrim(customer_charge_id) <> '')
        ),

    CONSTRAINT sell_evaluation_request_charge_fkey
        FOREIGN KEY (tenant_id, customer_charge_id)
        REFERENCES settlement_accounting.customer_charge (tenant_id, charge_id)
);
