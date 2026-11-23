-- 重量与体积单位按仓追加修订。新一版只作用于此后形成的发生项。
-- 更正版本沿用被更正那一版记下的单位，不读登记册的当前修订。
-- 已有的一行收成第 1 版。旧表留下，写入改走修订表。

CREATE TABLE warehouse_fulfillment.charge_measure_unit_revision (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    revision       integer     NOT NULL,
    weight_unit    text        NOT NULL,
    volume_unit    text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT charge_measure_unit_revision_pkey
        PRIMARY KEY (tenant_id, warehouse_code, revision),
    CONSTRAINT charge_measure_unit_revision_number CHECK (revision >= 1),
    CONSTRAINT charge_measure_unit_revision_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> ''
        AND btrim(weight_unit) <> '' AND btrim(volume_unit) <> ''
    )
);

INSERT INTO warehouse_fulfillment.charge_measure_unit_revision (
    tenant_id, warehouse_code, revision, weight_unit, volume_unit, registered_at
)
SELECT tenant_id, warehouse_code, 1, weight_unit, volume_unit, registered_at
  FROM warehouse_fulfillment.charge_measure_unit;

ALTER TABLE warehouse_fulfillment.charge_occurrence_version
    ADD COLUMN weight_unit text,
    ADD COLUMN volume_unit text,
    ADD CONSTRAINT charge_occurrence_version_units CHECK (
        (weight_unit IS NULL OR btrim(weight_unit) <> '')
        AND (volume_unit IS NULL OR btrim(volume_unit) <> '')
    );
