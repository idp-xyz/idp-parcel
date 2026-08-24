-- 申报单元本体（ADR-0073 决定一）：CONTEXT 要求「独立身份和可追溯组成，不能使用包裹、
-- 客户委托、集运单元、总单、运输舱单、监管舱单或班次直接替代」——此前单元只活在
-- declaration_submission.members 的 jsonb 快照里，没有独立行可挂案件维与替代关系。
--
-- 案件维随行落定且不可变更（决定二）：换案件即建立替代单元，本表没有 UPDATE 路径。
-- 「案件→该案下全部单元」按案件列加索引查询即得（决定三），不在案件聚合上加 units
-- 列，不建双向登记表——本表这一列是这条边唯一的存储。
--
-- replaces_unit_id 是替代申报单元的替代关系落点（CONTEXT「原申报单元不能继续使用但
-- 案件固定辖区、方向、程序和法定义务范围不变时，建立替代申报单元」）；首版允许为空
--（ADR-0073 Consequences），替代编排随后续票。
--
-- 单元内容（哪些包裹、什么程序）属业务事实，随提交链落；本表先于租户存在属机制半边。

CREATE TABLE customs_compliance.declaration_unit (
    tenant_id        text        NOT NULL,
    unit_id          text        NOT NULL,

    case_id          text        NOT NULL,
    procedure_ref    text        NOT NULL,
    members          jsonb       NOT NULL,
    formed_at        timestamptz NOT NULL,
    replaces_unit_id text,

    CONSTRAINT declaration_unit_pkey
        PRIMARY KEY (tenant_id, unit_id),
    -- 悬空案件在库上也进不来：用例已按标识反查核存在（ADR-0073 决定五），这里是
    -- 同一判断的库内防线，靠 customs_case_id_unique 撑住引用。
    CONSTRAINT declaration_unit_case_exists
        FOREIGN KEY (tenant_id, case_id)
        REFERENCES customs_compliance.customs_case (tenant_id, case_id),

    CONSTRAINT declaration_unit_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(unit_id) <> ''
            AND btrim(case_id) <> ''
            AND btrim(procedure_ref) <> ''
        ),
    -- 组成缺一不可（领域 FormDeclarationUnit 同门）。
    CONSTRAINT declaration_unit_members_present
        CHECK (jsonb_typeof(members) = 'array' AND jsonb_array_length(members) >= 1),
    CONSTRAINT declaration_unit_replacement_named
        CHECK (replaces_unit_id IS NULL OR btrim(replaces_unit_id) <> '')
);

-- 反向查询（案件→单元集）的索引：关闭核对将按它盘点该案下全部申报单元（ADR-0073
-- Consequences「关闭核对从此有结构可盘」）。
CREATE INDEX declaration_unit_case_idx
    ON customs_compliance.declaration_unit (tenant_id, case_id);
