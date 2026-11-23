-- 委托级销售计费。包裹那一路的 sell_evaluation_request 与 BUY 的 evaluation_request 不动。
-- 采用册不插任何租户的行：没有这一行就不触发。
-- 请求自然键是（租户、委托、计费时点、代次）。成员清单是形成这一拍时仍然有效且都已完成有效网络收寄的那些件。

CREATE TABLE settlement_accounting.shipment_billing_moment_adoption (
    tenant_id        text        NOT NULL,
    account_id       text        NOT NULL,
    billing_moment   text        NOT NULL,
    pricing_scope_id text,
    registered_at    timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT shipment_billing_moment_adoption_pkey
        PRIMARY KEY (tenant_id, account_id),

    CONSTRAINT shipment_billing_moment_adoption_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(account_id) <> ''
            AND (pricing_scope_id IS NULL OR btrim(pricing_scope_id) <> '')
        ),

    CONSTRAINT shipment_billing_moment_adoption_moment_closed
        CHECK (billing_moment IN ('SHIPMENT_MEMBERS_INTAKE_ADOPTED')),

    CONSTRAINT shipment_billing_moment_adoption_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id)
);

CREATE TABLE settlement_accounting.shipment_sell_evaluation_request (
    tenant_id           text        NOT NULL,
    request_id          text        NOT NULL,
    shipment_id         text        NOT NULL,
    member_ids          text[]      NOT NULL,
    account_id          text        NOT NULL,
    settlement_currency text        NOT NULL,
    pricing_scope_id    text        NOT NULL,
    pricing_scope_ids   text[]      NOT NULL,
    billing_moment      text        NOT NULL,
    source_fact_ref     text        NOT NULL,
    pricing_basis_at    timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL,
    state               text        NOT NULL,
    customer_charge_id  text,
    generation          integer     NOT NULL,
    previous_request_id text,
    resume_basis        text,
    stop_cause          text,
    stopped_at          timestamptz,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT shipment_sell_evaluation_request_pkey
        PRIMARY KEY (tenant_id, request_id),

    CONSTRAINT shipment_sell_evaluation_request_natural_key_generation
        UNIQUE (tenant_id, shipment_id, billing_moment, generation),

    CONSTRAINT shipment_sell_evaluation_request_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(request_id) <> ''
            AND btrim(shipment_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(settlement_currency) <> ''
            AND btrim(pricing_scope_id) <> ''
            AND btrim(source_fact_ref) <> ''
            AND cardinality(member_ids) > 0
            AND cardinality(pricing_scope_ids) > 0
            AND pricing_scope_ids[1] = pricing_scope_id
        ),

    CONSTRAINT shipment_sell_evaluation_request_moment_closed
        CHECK (billing_moment IN ('SHIPMENT_MEMBERS_INTAKE_ADOPTED')),

    CONSTRAINT shipment_sell_evaluation_request_lineage
        CHECK (
            generation >= 1
            AND (generation = 1) = (previous_request_id IS NULL)
            AND (generation = 1 OR resume_basis IS NOT NULL)
            AND (previous_request_id IS NULL OR btrim(previous_request_id) <> '')
            AND (resume_basis IS NULL OR btrim(resume_basis) <> '')
        ),

    CONSTRAINT shipment_sell_evaluation_request_state_closed
        CHECK (state IN ('SUBMITTED', 'FORMED', 'PENDING_JUDGMENT', 'UNRATABLE', 'CONFLICT', 'NOT_FORMED', 'ZERO_TOTAL')),

    CONSTRAINT shipment_sell_evaluation_request_charge_only_when_formed
        CHECK (
            (state = 'FORMED') = (customer_charge_id IS NOT NULL)
            AND (customer_charge_id IS NULL OR btrim(customer_charge_id) <> '')
        ),

    CONSTRAINT shipment_sell_evaluation_request_stop_paired
        CHECK (
            (state IN ('SUBMITTED', 'FORMED', 'ZERO_TOTAL') AND stop_cause IS NULL AND stopped_at IS NULL)
            OR (state IN ('PENDING_JUDGMENT', 'UNRATABLE', 'CONFLICT', 'NOT_FORMED')
                AND stop_cause IS NOT NULL AND btrim(stop_cause) <> '' AND stopped_at IS NOT NULL)
        ),

    CONSTRAINT shipment_sell_evaluation_request_charge_fkey
        FOREIGN KEY (tenant_id, customer_charge_id)
        REFERENCES settlement_accounting.customer_charge (tenant_id, charge_id)
);
