-- 限额、比例、免赔三项取值。先免赔、再比例、再限额的文法不在这张表里。
-- 金额规则版本册只存版本引用，这三项不写回去。不设修订。不登任何租户的行。

CREATE TABLE settlement_accounting.amount_grammar_parameter (
    tenant_id            text        NOT NULL,
    subject_kind         text        NOT NULL,
    subject_ref          text        NOT NULL,
    limit_minor          bigint      NOT NULL,
    ratio_basis_points   integer     NOT NULL,
    deductible_minor     bigint      NOT NULL,
    registered_at        timestamptz NOT NULL,
    inserted_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT amount_grammar_parameter_pkey
        PRIMARY KEY (tenant_id, subject_kind, subject_ref),

    CONSTRAINT amount_grammar_parameter_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(subject_ref) <> ''
        ),

    CONSTRAINT amount_grammar_parameter_kind_closed
        CHECK (subject_kind IN ('CLAIM_RULE', 'RECOVERY_CONTRACT')),

    CONSTRAINT amount_grammar_parameter_values
        CHECK (
            limit_minor >= 0
            AND deductible_minor >= 0
            AND ratio_basis_points BETWEEN 0 AND 10000
        )
);
