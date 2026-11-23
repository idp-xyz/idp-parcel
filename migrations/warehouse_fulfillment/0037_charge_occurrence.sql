-- 仓储作业收费发生项（UC-WF-019）。不带金额。一种来源事实一份，更正只追加版本。
-- 重量体积单位与库龄口径是租户取值，本迁移不种任何行。

CREATE TABLE warehouse_fulfillment.charge_measure_unit (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    weight_unit    text        NOT NULL,
    volume_unit    text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT charge_measure_unit_pkey PRIMARY KEY (tenant_id, warehouse_code),
    CONSTRAINT charge_measure_unit_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(weight_unit) <> '' AND btrim(volume_unit) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.charge_aging_basis (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    basis          text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT charge_aging_basis_pkey PRIMARY KEY (tenant_id, warehouse_code),
    CONSTRAINT charge_aging_basis_kind CHECK (basis IN ('ACTUAL_BATCH', 'THEORETICAL_FIFO')),
    CONSTRAINT charge_aging_basis_not_blank CHECK (btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '')
);

CREATE TABLE warehouse_fulfillment.charge_occurrence (
    tenant_id        text    NOT NULL,
    occurrence_id    text    NOT NULL,
    kind             text    NOT NULL,
    owner_account_id text    NOT NULL,
    warehouse_code   text    NOT NULL,
    source_fact      text    NOT NULL,
    head_version     integer NOT NULL,

    CONSTRAINT charge_occurrence_pkey PRIMARY KEY (tenant_id, occurrence_id),
    CONSTRAINT charge_occurrence_source UNIQUE (tenant_id, kind, source_fact),
    CONSTRAINT charge_occurrence_kind CHECK (kind IN (
        'INBOUND_HANDLING', 'OUTBOUND_HANDLING', 'STORAGE', 'RETURN_HANDLING',
        'VALUE_ADDED', 'PROCESSING', 'DESTRUCTION'
    )),
    CONSTRAINT charge_occurrence_head CHECK (head_version >= 1),
    CONSTRAINT charge_occurrence_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(occurrence_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(source_fact) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.charge_occurrence_version (
    tenant_id      text        NOT NULL,
    occurrence_id  text        NOT NULL,
    version        integer     NOT NULL,
    supersedes     integer,
    correction     text        NOT NULL,
    business_time  timestamptz NOT NULL,
    measures       jsonb       NOT NULL,
    attributes     jsonb       NOT NULL,
    lines          jsonb       NOT NULL,

    CONSTRAINT charge_occurrence_version_pkey PRIMARY KEY (tenant_id, occurrence_id, version),
    CONSTRAINT charge_occurrence_version_occurrence_fkey
        FOREIGN KEY (tenant_id, occurrence_id)
        REFERENCES warehouse_fulfillment.charge_occurrence (tenant_id, occurrence_id),
    CONSTRAINT charge_occurrence_version_number CHECK (version >= 1),
    CONSTRAINT charge_occurrence_version_correction CHECK (correction IN ('ORIGINAL', 'RESTATE', 'VOID')),
    CONSTRAINT charge_occurrence_version_void_shape CHECK (
        correction <> 'VOID' OR (measures = '[]'::jsonb AND lines = '[]'::jsonb)
    ),
    CONSTRAINT charge_occurrence_version_link CHECK (
        (correction = 'ORIGINAL' AND supersedes IS NULL AND version = 1)
        OR (correction <> 'ORIGINAL' AND supersedes IS NOT NULL AND supersedes >= 1 AND version = supersedes + 1)
    )
);

CREATE UNIQUE INDEX charge_occurrence_version_one_successor
    ON warehouse_fulfillment.charge_occurrence_version (tenant_id, occurrence_id, supersedes)
    WHERE supersedes IS NOT NULL;

CREATE TABLE warehouse_fulfillment.charge_service_project (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    project_name   text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT charge_service_project_pkey PRIMARY KEY (tenant_id, warehouse_code, project_name),
    CONSTRAINT charge_service_project_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '' AND btrim(project_name) <> ''
    )
);
