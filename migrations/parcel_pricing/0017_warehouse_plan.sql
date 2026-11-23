-- 仓储计费方案（UC-PP-001，ADR-0377）。规范化版本 PPC-8，不是基础版。不种任何租户的行。

CREATE TABLE parcel_pricing.warehouse_plan (
    tenant_id      text        NOT NULL,
    plan_id        text        NOT NULL,
    version        text        NOT NULL,
    scope          text        NOT NULL,
    currency       text        NOT NULL,
    effective_from timestamptz NOT NULL,
    content_digest text        NOT NULL,
    document       jsonb       NOT NULL,

    CONSTRAINT warehouse_plan_pkey PRIMARY KEY (tenant_id, plan_id, version),
    CONSTRAINT warehouse_plan_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(plan_id) <> '' AND btrim(version) <> ''
        AND btrim(scope) <> '' AND btrim(currency) <> '' AND btrim(content_digest) <> ''
    )
);

CREATE TABLE parcel_pricing.warehouse_evaluation (
    tenant_id      text    NOT NULL,
    evaluation_id  text    NOT NULL,
    occurrence_id  text    NOT NULL,
    version        integer NOT NULL,
    plan_id        text    NOT NULL,
    status         text    NOT NULL,
    total_minor    bigint  NOT NULL,
    currency       text    NOT NULL,
    document       jsonb   NOT NULL,

    CONSTRAINT warehouse_evaluation_pkey PRIMARY KEY (tenant_id, evaluation_id),
    CONSTRAINT warehouse_evaluation_status CHECK (status IN ('COMPLETED', 'ZERO')),
    CONSTRAINT warehouse_evaluation_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(evaluation_id) <> '' AND btrim(occurrence_id) <> ''
        AND btrim(plan_id) <> '' AND btrim(currency) <> ''
    )
);
