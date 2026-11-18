-- 实际测量登记册与作废事实。
--
-- 一条测量落下之后不改数字。称错了、复称了，是再登一条，再写一条作废指回旧的来源身份。
-- 仍有效的读法是「没有作废指过来」，不在测量行上翻一个状态位：翻位就是覆盖。
--
-- 发生时间是现场报的，记录时刻是服务端落库的时刻，两列分开。来源身份是幂等键；
-- 同一键异内容靠内容摘要认，不靠后到的那条覆盖先到的。
--
-- 一条测量是一次称重量方：毛重与长宽高都必填，各带自己的单位。只称了重、没量方的那一条不收。复测是再登一条
-- 完整的。录入人与原件成对：设备上报两列都空，手工录入两列都在。本迁移不种任何租户的行。

CREATE TABLE node_operations.actual_measurement (
    tenant_id         text        NOT NULL,
    source_id         text        NOT NULL,
    content_digest    text        NOT NULL,
    object_kind       text        NOT NULL,
    object_id         text        NOT NULL,
    node_ref          text        NOT NULL,
    location_ref      text        NOT NULL,
    basis_ref         text        NOT NULL,
    weight_value      text        NOT NULL,
    weight_unit       text        NOT NULL,
    length_value      text        NOT NULL,
    width_value       text        NOT NULL,
    height_value      text        NOT NULL,
    dimension_unit    text        NOT NULL,
    performed_by      text        NOT NULL,
    evidence          text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    recorded_at       timestamptz NOT NULL,
    entered_by        text,
    original_document text,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT actual_measurement_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT actual_measurement_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(object_id) <> ''
            AND btrim(node_ref) <> ''
            AND btrim(location_ref) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(performed_by) <> ''
            AND btrim(evidence) <> ''
        ),

    CONSTRAINT actual_measurement_object_kind_closed
        CHECK (object_kind IN ('HANDLING_UNIT', 'CONSOLIDATION_UNIT')),

    CONSTRAINT actual_measurement_weight_and_dimensions_present
        CHECK (
            btrim(weight_value) <> ''
            AND btrim(weight_unit) <> ''
            AND btrim(length_value) <> ''
            AND btrim(width_value) <> ''
            AND btrim(height_value) <> ''
            AND btrim(dimension_unit) <> ''
        ),

    CONSTRAINT actual_measurement_manual_entry_paired
        CHECK ((entered_by IS NULL) = (original_document IS NULL)),

    CONSTRAINT actual_measurement_manual_entry_not_blank
        CHECK (
            (entered_by IS NULL OR btrim(entered_by) <> '')
            AND (original_document IS NULL OR btrim(original_document) <> '')
        )
);

CREATE INDEX actual_measurement_by_object
    ON node_operations.actual_measurement (tenant_id, object_kind, object_id, occurred_at, source_id);

-- 一条测量至多被作废一次。第二次作废是另一条来源，唯一约束拦住，不改已经写下的那条。
CREATE TABLE node_operations.measurement_invalidation (
    tenant_id         text        NOT NULL,
    source_id         text        NOT NULL,
    content_digest    text        NOT NULL,
    target_source_id  text        NOT NULL,
    reason            text        NOT NULL,
    performed_by      text        NOT NULL,
    evidence          text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    recorded_at       timestamptz NOT NULL,
    entered_by        text,
    original_document text,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT measurement_invalidation_pkey
        PRIMARY KEY (tenant_id, source_id),

    CONSTRAINT measurement_invalidation_target_unique
        UNIQUE (tenant_id, target_source_id),

    CONSTRAINT measurement_invalidation_target_fk
        FOREIGN KEY (tenant_id, target_source_id)
        REFERENCES node_operations.actual_measurement (tenant_id, source_id),

    CONSTRAINT measurement_invalidation_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(source_id) <> ''
            AND btrim(content_digest) <> ''
            AND btrim(target_source_id) <> ''
            AND btrim(reason) <> ''
            AND btrim(performed_by) <> ''
            AND btrim(evidence) <> ''
        ),

    CONSTRAINT measurement_invalidation_not_self
        CHECK (source_id <> target_source_id),

    CONSTRAINT measurement_invalidation_manual_entry_paired
        CHECK ((entered_by IS NULL) = (original_document IS NULL)),

    CONSTRAINT measurement_invalidation_manual_entry_not_blank
        CHECK (
            (entered_by IS NULL OR btrim(entered_by) <> '')
            AND (original_document IS NULL OR btrim(original_document) <> '')
        )
);
