-- 两本规则登记册：外部响应的解释规则，与「此监管范围要不要建案」的判断规则。
--
-- 两者都是「无行=未决」而不是「无行=否」。解释规则缺席时不得用默认口径猜测监管语义
-- （CONTEXT 硬句 186 要求逐条外部结果保存实际采用的解释规则）；建案规则缺席时是
-- 未决，不是「不要求建案」——把未登记读成否，就会让该建的案不建而无人察觉。
--
-- 规则正文属实例半边（真实监管程序与服务合同才定得出），登记册本身属机制半边。

CREATE TABLE customs_compliance.interpretation_rule (
    tenant_id   text NOT NULL,
    result_layer text NOT NULL,

    rule_ref    text NOT NULL,

    CONSTRAINT interpretation_rule_pkey
        PRIMARY KEY (tenant_id, result_layer),

    CONSTRAINT interpretation_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(rule_ref) <> ''
        ),
    -- 外部监管结果的封闭六层（CONTEXT 硬句 185 分层保存）。库内钉住这一集合：多出
    -- 一个「清关成功」之类的合成层，正是该硬句要禁的东西。
    CONSTRAINT interpretation_rule_layer_closed
        CHECK (result_layer IN (
            'REGULATORY_RECEIPT',
            'BUSINESS_ACCEPTANCE',
            'PROCESS_DECISION',
            'ASSESSED_DUTY',
            'RELEASE_RESULT',
            'DISPOSITION_DECISION'
        ))
);

CREATE TABLE customs_compliance.case_requirement_rule (
    tenant_id        text    NOT NULL,
    jurisdiction_ref text    NOT NULL,
    direction        text    NOT NULL,
    procedure_ref    text    NOT NULL,

    required         boolean NOT NULL,
    basis            text    NOT NULL,

    CONSTRAINT case_requirement_rule_pkey
        PRIMARY KEY (tenant_id, jurisdiction_ref, direction, procedure_ref),

    -- 依据对两个取值都必填：说不出依据的「不要求建案」与「规则没登记」分不开，而
    -- 这两者的续办动作完全不同（前者照常推进，后者等实例参数）。
    CONSTRAINT case_requirement_rule_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(jurisdiction_ref) <> ''
            AND btrim(procedure_ref) <> ''
            AND btrim(basis) <> ''
        ),
    CONSTRAINT case_requirement_rule_direction_closed
        CHECK (direction IN ('IMPORT', 'EXPORT'))
);
