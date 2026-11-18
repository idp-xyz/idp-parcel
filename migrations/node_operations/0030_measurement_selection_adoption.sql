-- 当前有效实测的形态采用。一租户一行。没有这一行就是没采用：多于一条仍有效实测时不定一条。
-- 内置形态只有「发生时间最近的一条仍有效」。本迁移不种任何租户的行。

CREATE TABLE node_operations.measurement_selection_adoption (
    tenant_id   text        NOT NULL,
    form        text        NOT NULL,
    basis       text        NOT NULL,
    recorded_at timestamptz NOT NULL,
    inserted_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT measurement_selection_adoption_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT measurement_selection_adoption_form_closed
        CHECK (form = 'LATEST_STILL_VALID_BY_OCCURRED_AT'),

    CONSTRAINT measurement_selection_adoption_text_present
        CHECK (btrim(tenant_id) <> '' AND btrim(basis) <> '')
);
