-- 一条分摊规则版本选用哪套内置分法。按重、按件、按收入的展开不在这张表里。
-- 分摊规则适用表只存版本引用，分法不写回去。各对象的权重也不在这里。不设修订。不登任何租户的行。

CREATE TABLE settlement_accounting.allocation_form_choice (
    tenant_id     text        NOT NULL,
    rule_version  text        NOT NULL,
    form          text        NOT NULL,
    registered_at timestamptz NOT NULL,
    inserted_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT allocation_form_choice_pkey
        PRIMARY KEY (tenant_id, rule_version),

    CONSTRAINT allocation_form_choice_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(rule_version) <> ''
        ),

    CONSTRAINT allocation_form_choice_form_closed
        CHECK (form IN ('BY_WEIGHT', 'BY_PIECE', 'BY_REVENUE', 'NOT_APPLICABLE'))
);
