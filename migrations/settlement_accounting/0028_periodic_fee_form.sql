-- 周期费用形态与数值。最低消费补差、保底量不足、阶梯返利的算法不在这两张表里。
-- 不设修订。不登任何租户的行。价卡上的最低收费不写在这里。

CREATE TABLE settlement_accounting.periodic_fee_form (
    tenant_id           text        NOT NULL,
    rule_ref            text        NOT NULL,
    form_kind           text        NOT NULL,
    minimum_minor       bigint,
    committed_quantity  bigint,
    rate_minor          bigint,
    registered_at       timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT periodic_fee_form_pkey
        PRIMARY KEY (tenant_id, rule_ref),

    CONSTRAINT periodic_fee_form_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(rule_ref) <> ''),

    CONSTRAINT periodic_fee_form_kind_closed
        CHECK (form_kind IN ('MINIMUM_SPEND', 'VOLUME_FLOOR', 'TIERED_REBATE', 'NOT_APPLICABLE')),

    CONSTRAINT periodic_fee_form_shaped
        CHECK (
            (form_kind = 'MINIMUM_SPEND'
                AND minimum_minor IS NOT NULL AND minimum_minor >= 0
                AND committed_quantity IS NULL AND rate_minor IS NULL)
            OR (form_kind = 'VOLUME_FLOOR'
                AND committed_quantity IS NOT NULL AND committed_quantity >= 0
                AND rate_minor IS NOT NULL AND rate_minor >= 0
                AND minimum_minor IS NULL)
            OR (form_kind = 'TIERED_REBATE'
                AND minimum_minor IS NULL AND committed_quantity IS NULL AND rate_minor IS NULL)
            OR (form_kind = 'NOT_APPLICABLE'
                AND minimum_minor IS NULL AND committed_quantity IS NULL AND rate_minor IS NULL)
        )
);

CREATE TABLE settlement_accounting.periodic_fee_tier (
    tenant_id           text    NOT NULL,
    rule_ref            text    NOT NULL,
    tier_ordinal        integer NOT NULL,
    upper_minor         bigint,
    rate_basis_points   integer NOT NULL,

    CONSTRAINT periodic_fee_tier_pkey
        PRIMARY KEY (tenant_id, rule_ref, tier_ordinal),

    CONSTRAINT periodic_fee_tier_form_fk
        FOREIGN KEY (tenant_id, rule_ref)
        REFERENCES settlement_accounting.periodic_fee_form (tenant_id, rule_ref),

    CONSTRAINT periodic_fee_tier_ordinal_positive
        CHECK (tier_ordinal > 0),

    CONSTRAINT periodic_fee_tier_rate
        CHECK (rate_basis_points BETWEEN 0 AND 10000),

    CONSTRAINT periodic_fee_tier_upper
        CHECK (upper_minor IS NULL OR upper_minor > 0)
);
