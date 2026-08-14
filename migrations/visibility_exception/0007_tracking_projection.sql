-- 全程追踪投影：键=租户+包裹，库只管当前版。重派生换版本改同一行，历史由
-- prior_version 指回承担。归类条目进 jsonb——只引用源事实与映射版本，不复制
-- 源事实内容。租户是最高数据隔离边界（ADR-0003）。

CREATE TABLE visibility_exception.tracking_projection (
    tenant_id     text        NOT NULL,
    parcel_ref    text        NOT NULL,

    version_id    text        NOT NULL,
    derived_at    timestamptz NOT NULL,
    prior_version text,
    entries       jsonb       NOT NULL,

    CONSTRAINT tracking_projection_pkey PRIMARY KEY (tenant_id, parcel_ref),

    CONSTRAINT tracking_projection_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(version_id) <> ''
        ),
    CONSTRAINT tracking_projection_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version_id)
        ),
    CONSTRAINT tracking_projection_entries_present
        CHECK (jsonb_typeof(entries) = 'array' AND jsonb_array_length(entries) >= 1)
);
