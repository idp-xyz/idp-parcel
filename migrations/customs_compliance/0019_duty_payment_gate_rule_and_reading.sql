-- 放行门禁里「税费付款」那一道的读法（票 sa-cc/06，ADR-0137 决定三）：规则登在门禁目录、门禁记录带
-- 三态原值与核对版本引用。CONTEXT 的话：「税费支付是否是放行前置条件，取决于当前监管程序的适用规则；
-- 本上下文不得统一假设『先税后放』或『先放后税』」——折法因此不是代码常量，是这张表里登进来的一行；
-- 表上没有任何默认行，规则的取值属实例半边 `PAR-CUS-0x`。

-- 规则行与门禁目录同键、外键到目录行：规则与认定是同一道门的两种登法（0008 的 gate_condition_finding
-- 登认定），再开一册是同一把键的第二张表——所以它挂在目录这一册下，一个目录行至多一条规则。规则正文
-- 两形之一：not_a_precondition 为真即「税费付款不构成本动作在本边界的前置条件」，三个接受集合皆空；
-- 否则三个接受集合各非空，jsonb 数组存封闭词。`待确认` / `冲突` 不可登为接受（UC-CC-003「未知不能当作
-- 可选、不适用、有效或已解除」）——CHECK 用 jsonb 包含只放得进各轴可接受的词，PENDING / CONFLICTING
-- 在库上就进不来。改规则不 UPDATE：走复核另登，已按旧规则折出的门禁记录引用的是那条规则说过的话。
CREATE TABLE customs_compliance.gate_condition_duty_payment_rule (
    tenant_id          text        NOT NULL,
    scope_ref          text        NOT NULL,
    action             text        NOT NULL,
    boundary_ref       text        NOT NULL,

    not_a_precondition boolean     NOT NULL,
    accept_coverage    jsonb       NOT NULL,
    accept_delta       jsonb       NOT NULL,
    accept_validity    jsonb       NOT NULL,
    registered_at      timestamptz NOT NULL,

    CONSTRAINT gate_condition_duty_payment_rule_pkey
        PRIMARY KEY (tenant_id, scope_ref, action, boundary_ref),
    CONSTRAINT gate_condition_duty_payment_rule_catalog_exists
        FOREIGN KEY (tenant_id, scope_ref, action, boundary_ref)
        REFERENCES customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref),

    CONSTRAINT gate_condition_duty_payment_rule_sets_are_arrays
        CHECK (
            jsonb_typeof(accept_coverage) = 'array'
            AND jsonb_typeof(accept_delta) = 'array'
            AND jsonb_typeof(accept_validity) = 'array'
        ),
    -- 两形互斥：不构成前置条件则三个集合皆空；否则三个集合各非空（空集合等于「永不满足」，那是常量不是规则）。
    CONSTRAINT gate_condition_duty_payment_rule_shape
        CHECK (
            (not_a_precondition
                AND jsonb_array_length(accept_coverage) = 0
                AND jsonb_array_length(accept_delta) = 0
                AND jsonb_array_length(accept_validity) = 0)
            OR (NOT not_a_precondition
                AND jsonb_array_length(accept_coverage) >= 1
                AND jsonb_array_length(accept_delta) >= 1
                AND jsonb_array_length(accept_validity) >= 1)
        ),
    -- 各轴可接受的词封闭，与 domain.DutyCoverage / DutyDelta / DutyFactValidity 的 String 同词；差额的 PENDING 与
    -- 有效性的 CONFLICTING / PENDING 不在集合里——它们不是能被接受的答案。
    CONSTRAINT gate_condition_duty_payment_rule_coverage_closed
        CHECK (accept_coverage <@ '["NONE", "PARTIAL", "COVERED"]'::jsonb),
    CONSTRAINT gate_condition_duty_payment_rule_delta_closed
        CHECK (accept_delta <@ '["NO_DELTA", "SHORT", "EXCESS"]'::jsonb),
    CONSTRAINT gate_condition_duty_payment_rule_validity_closed
        CHECK (accept_validity <@ '["VALID", "INVALIDATED"]'::jsonb)
);

-- 门禁记录带「税费付款」那一道的读数：按规则折出的判断、三态原值、核对版本引用（税费引用 + 资金事实引用 +
-- 版本指纹；范围与门禁同一个，不重复带）。引用不快照（票 sa-cc/06 裁决 2），三态原值另带是为了「覆盖状态、
-- 差额状态和有效性状态分别表达」——没有合成布尔列。七列同生同灭：没挂读数的版本（不构成前置条件、或本
-- 迁移之前的旧版）七列皆空；挂了就七列皆非空。读数里不会有 PENDING / CONFLICTING——那一格是未决、不入册。
ALTER TABLE customs_compliance.gate_verification
    ADD COLUMN duty_state          text,
    ADD COLUMN duty_coverage       text,
    ADD COLUMN duty_delta          text,
    ADD COLUMN duty_validity       text,
    ADD COLUMN duty_ref            text,
    ADD COLUMN funds_ref           text,
    ADD COLUMN duty_version_digest text,
    ADD CONSTRAINT gate_verification_duty_reading_paired
        CHECK (
            (duty_state IS NULL AND duty_coverage IS NULL AND duty_delta IS NULL AND duty_validity IS NULL
                AND duty_ref IS NULL AND funds_ref IS NULL AND duty_version_digest IS NULL)
            OR (duty_state IS NOT NULL AND duty_coverage IS NOT NULL AND duty_delta IS NOT NULL AND duty_validity IS NOT NULL
                AND duty_ref IS NOT NULL AND funds_ref IS NOT NULL AND duty_version_digest IS NOT NULL
                AND btrim(duty_ref) <> '' AND btrim(funds_ref) <> '' AND btrim(duty_version_digest) <> '')
        ),
    ADD CONSTRAINT gate_verification_duty_state_closed
        CHECK (duty_state IS NULL OR duty_state IN ('MET', 'UNMET')),
    ADD CONSTRAINT gate_verification_duty_coverage_closed
        CHECK (duty_coverage IS NULL OR duty_coverage IN ('NONE', 'PARTIAL', 'COVERED')),
    ADD CONSTRAINT gate_verification_duty_delta_closed
        CHECK (duty_delta IS NULL OR duty_delta IN ('NO_DELTA', 'SHORT', 'EXCESS')),
    ADD CONSTRAINT gate_verification_duty_validity_closed
        CHECK (duty_validity IS NULL OR duty_validity IN ('VALID', 'INVALIDATED'));
