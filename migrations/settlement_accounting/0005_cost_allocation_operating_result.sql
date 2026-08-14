-- 成本分摊与经营结果快照。
--
-- cost_allocation 主键取（租户+分摊）：Save 登记，Replace 换版本（份额/规则/回指），
-- 来源金额与身份在 INSERT 冻结。份额数组可空（全额未分摊）。
--
-- operating_result 主键取（租户+口径+账期+基准）：同一口径一版一登，Replace 换版本
-- 回指前身，不改口径三件。组成非空；毛利由读回重建门重算。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL；jsonb 列先验 IS NOT NULL 再取类型与长度。

CREATE TABLE settlement_accounting.cost_allocation (
    tenant_id          text        NOT NULL,
    allocation_id      text        NOT NULL,

    source_ref         text        NOT NULL,
    source_minor       bigint      NOT NULL,
    currency           text        NOT NULL,
    rule_ref           text        NOT NULL,
    portions           jsonb       NOT NULL,
    unallocated_minor  bigint      NOT NULL,
    version            text        NOT NULL,
    allocated_at       timestamptz NOT NULL,
    corrects           text,
    content_digest     text        NOT NULL,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT cost_allocation_pkey
        PRIMARY KEY (tenant_id, allocation_id),

    CONSTRAINT cost_allocation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(allocation_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(rule_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT cost_allocation_source_positive
        CHECK (source_minor > 0),

    CONSTRAINT cost_allocation_unallocated_range
        CHECK (unallocated_minor >= 0 AND unallocated_minor <= source_minor),

    CONSTRAINT cost_allocation_portions_shaped
        CHECK (
            portions IS NOT NULL
            AND jsonb_typeof(portions) = 'array'
        ),

    CONSTRAINT cost_allocation_corrects_independent
        CHECK (corrects IS NULL OR (btrim(corrects) <> '' AND corrects <> version))
);

CREATE TABLE settlement_accounting.operating_result (
    tenant_id      text        NOT NULL,
    scope_ref      text        NOT NULL,
    period_ref     text        NOT NULL,
    basis          text        NOT NULL,

    currency       text        NOT NULL,
    components     jsonb       NOT NULL,
    margin_minor   bigint      NOT NULL,
    version        text        NOT NULL,
    as_of          timestamptz NOT NULL,
    corrects       text,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT operating_result_pkey
        PRIMARY KEY (tenant_id, scope_ref, period_ref, basis),

    CONSTRAINT operating_result_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(period_ref) <> ''
            AND btrim(basis) <> ''
            AND btrim(currency) <> ''
            AND btrim(version) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT operating_result_basis_closed
        CHECK (basis IN ('ESTIMATED', 'CONFIRMED', 'SETTLED')),

    CONSTRAINT operating_result_components_shaped
        CHECK (
            components IS NOT NULL
            AND jsonb_typeof(components) = 'array'
            AND jsonb_array_length(components) > 0
        ),

    CONSTRAINT operating_result_corrects_independent
        CHECK (corrects IS NULL OR (btrim(corrects) <> '' AND corrects <> version))
);
