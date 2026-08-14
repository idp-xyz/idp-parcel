-- 处置执行核对：键=租户+决定+事实集指纹。同一决定加同一事实集只出一版；新事实
-- 到达换指纹换版（写入代数同 ADR-0031）。行内没有义务终结或案件关闭列——核对
-- 完成不自动终结监管义务。决定本体与执行事实引用进列/jsonb，读回经
-- VerifyDispositionExecution 整门重验。

CREATE TABLE customs_compliance.disposition_verification (
    tenant_id         text        NOT NULL,
    decision_id       text        NOT NULL,
    facts_digest      text        NOT NULL,

    authority_ref     text        NOT NULL,
    action_ref        text        NOT NULL,
    scope_ref         text        NOT NULL,
    quantity_provided boolean     NOT NULL,
    quantity_units    integer     NOT NULL,
    received_at       timestamptz NOT NULL,
    facts             jsonb       NOT NULL,
    conclusion        text        NOT NULL,
    executed_units    integer     NOT NULL,
    verified_at       timestamptz NOT NULL,

    CONSTRAINT disposition_verification_pkey
        PRIMARY KEY (tenant_id, decision_id, facts_digest),

    CONSTRAINT disposition_verification_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(facts_digest) <> ''
            AND btrim(authority_ref) <> ''
            AND btrim(action_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),
    CONSTRAINT disposition_verification_quantity_shape
        CHECK (
            quantity_units >= 0
            AND (NOT quantity_provided OR quantity_units > 0)
        ),
    CONSTRAINT disposition_verification_facts_shaped
        CHECK (jsonb_typeof(facts) = 'array'),
    CONSTRAINT disposition_verification_conclusion_closed
        CHECK (conclusion IN (
            'COVERED',
            'PARTIALLY_COVERED',
            'DEVIATION',
            'FACT_CONFLICT',
            'EVIDENCE_INSUFFICIENT'
        )),
    CONSTRAINT disposition_verification_executed_non_negative
        CHECK (executed_units >= 0)
);
