-- 放行门禁里「实测与提交版本毛重差异」那一道的读法：规则登在门禁目录、门禁记录带逐成员读数。
-- 容差的数值与单位是租户取值，这张表上没有任何默认行；没登规则时门禁如实停在未配置，不取零容差也不放过。

-- 规则行与门禁目录同键、外键到目录行：规则与认定是同一道门的两种登法，一个目录行至多一条规则。
-- 规则正文两形之一：not_a_precondition 为真即「毛重差异不构成本动作在本边界的前置条件」，容差与单位皆空；
-- 否则容差与重量单位都在。重量单位与 refdata 的重量类同一份封闭词。改规则不 UPDATE：走复核另登。
CREATE TABLE customs_compliance.gate_condition_weight_discrepancy_rule (
    tenant_id          text        NOT NULL,
    scope_ref          text        NOT NULL,
    action             text        NOT NULL,
    boundary_ref       text        NOT NULL,

    not_a_precondition boolean     NOT NULL,
    max_delta          text,
    weight_unit        text,
    registered_at      timestamptz NOT NULL,

    CONSTRAINT gate_condition_weight_discrepancy_rule_pkey
        PRIMARY KEY (tenant_id, scope_ref, action, boundary_ref),
    CONSTRAINT gate_condition_weight_discrepancy_rule_catalog_exists
        FOREIGN KEY (tenant_id, scope_ref, action, boundary_ref)
        REFERENCES customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref),

    CONSTRAINT gate_condition_weight_discrepancy_rule_shape
        CHECK (
            (not_a_precondition AND max_delta IS NULL AND weight_unit IS NULL)
            OR (NOT not_a_precondition
                AND max_delta IS NOT NULL AND btrim(max_delta) <> ''
                AND weight_unit IS NOT NULL
                AND weight_unit IN ('KG', 'G', 'MG', 'T', 'LB', 'OZ'))
        )
);

-- 不构成前置条件或本列加上之前的旧版，这一列为 NULL。挂了读数时存结构化对象，读回经领域重建。
ALTER TABLE customs_compliance.gate_verification
    ADD COLUMN weight_discrepancy jsonb,
    ADD CONSTRAINT gate_verification_weight_discrepancy_object
        CHECK (weight_discrepancy IS NULL OR jsonb_typeof(weight_discrepancy) = 'object');
