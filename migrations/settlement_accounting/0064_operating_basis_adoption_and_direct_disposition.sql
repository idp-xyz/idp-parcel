-- 经营口径采用与直接成本答不出毛利时的当前答案（票 commercial-four-module-requirements/05）。
--
-- operating_basis_adoption 一户一行：显式采用预估、已确认，或两者都采用。两个布尔都为假
-- 不设这一行——那与「没登记」是同一句话，而没登记要由读口答未配置。本迁移不插入任何租户的行，
-- 也不给演示租户写默认采用；演示租户的采用只在 scripts/demo-seeds 里显式登记。
--
-- direct_operating_disposition 记下某件包裹在某个已采用口径下答不出毛利的当前答案：判不出、
-- 成本不全、待判断。它不是 operating_result：那边的毛利只能由组成项派生，缺一侧或成本不全
-- 时写一行毛利会把缺的那侧当成零。已结算口径不进这张表。

CREATE TABLE settlement_accounting.operating_basis_adoption (
    tenant_id       text        NOT NULL,
    adopt_estimated boolean     NOT NULL,
    adopt_confirmed boolean     NOT NULL,
    recorded_at     timestamptz NOT NULL,

    CONSTRAINT operating_basis_adoption_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT operating_basis_adoption_tenant_not_blank
        CHECK (btrim(tenant_id) <> ''),

    CONSTRAINT operating_basis_adoption_names_a_basis
        CHECK (adopt_estimated OR adopt_confirmed)
);

CREATE TABLE settlement_accounting.direct_operating_disposition (
    tenant_id    text        NOT NULL,
    scope_ref    text        NOT NULL,
    basis        text        NOT NULL,
    disposition  text        NOT NULL,
    recorded_at  timestamptz NOT NULL,

    CONSTRAINT direct_operating_disposition_pkey
        PRIMARY KEY (tenant_id, scope_ref, basis),

    CONSTRAINT direct_operating_disposition_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(disposition) <> ''
        ),

    CONSTRAINT direct_operating_disposition_basis_closed
        CHECK (basis IN ('ESTIMATED', 'CONFIRMED')),

    CONSTRAINT direct_operating_disposition_closed
        CHECK (disposition IN ('INDETERMINATE', 'INCOMPLETE_COST', 'PENDING_JUDGMENT'))
);
