-- 运输收费发生项登记册（tf-unwired-seven/04，ADR-0098）。
--
-- **本册没有金额列，也没有币种列，这不是遗漏。** 领域类型 `TransportChargeOccurrence` 上
-- 就没有那些字段：发生项是「可能依据供应商协议形成外部运输成本的业务事实范围」，不是价格、
-- 不是供应商预期成本、不是账单主张、不是审核应付。金额由 `parcel-pricing` 形成 BUY 纯评价、
-- `settlement-accounting` 判断费用金额责任。**加一列 amount_minor 等于把那两个上下文的所有权
-- 搬到这里，而搬过来之后没有任何东西会报警。**
--
-- 分两张表：成员是一个对象清单（CONTEXT 要求发生项固定其对象范围），压进数组或 JSON 就
-- 查不动也约束不住。
--
-- 主键含有效性版本：更正**换版本不删原项**（CONTEXT「来源更正……保留原发生项并形成失效、
-- 替代或范围更正关系；结算依据新的有效性追加调整，不删除原成本」）。两代因此天然共存，
-- corrects_version 回指前身。

CREATE TABLE transport_fulfillment.transport_charge_occurrence (
    tenant_id        text        NOT NULL,
    occurrence_ref   text        NOT NULL,
    validity_version text        NOT NULL,

    journey_ref      text        NOT NULL,
    legal_entity_ref text        NOT NULL,
    provider_ref     text        NOT NULL,
    agreement_ref    text        NOT NULL,
    reason           text        NOT NULL,
    fact_basis       text        NOT NULL,
    scope_ref        text        NOT NULL,
    quantity         bigint      NOT NULL,
    unit_ref         text        NOT NULL,
    occurred_at      timestamptz NOT NULL,

    corrects_version text,
    revision_kind    text,
    revision_basis   text,
    revised_at       timestamptz,

    recorded_at      timestamptz NOT NULL,

    CONSTRAINT transport_charge_occurrence_pkey
        PRIMARY KEY (tenant_id, occurrence_ref, validity_version),

    CONSTRAINT transport_charge_occurrence_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(occurrence_ref) <> ''
            AND btrim(validity_version) <> ''
            AND btrim(journey_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(fact_basis) <> ''
            AND btrim(unit_ref) <> ''
        ),

    -- 采购三件必备。它们正是 ADR-0098 决定二说的「不由本上下文推导」的那三件：
    -- 自营履约没有协议快照，因此那种情况下**根本不形成发生项**，而不是留空落一行。
    CONSTRAINT transport_charge_occurrence_procurement_required
        CHECK (
            btrim(legal_entity_ref) <> ''
            AND btrim(provider_ref) <> ''
            AND btrim(agreement_ref) <> ''
        ),

    -- 发生原因封闭四值，镜像领域的 ChargeOccurrenceReason。
    CONSTRAINT transport_charge_occurrence_reason_closed
        CHECK (reason IN ('BOOKING', 'CANCELLATION', 'FAILED_ATTEMPT', 'ACTUAL_FULFILLMENT')),

    CONSTRAINT transport_charge_occurrence_quantity_positive
        CHECK (quantity > 0),

    -- 修订四件同在或同缺。Revision() 按 revised_at 是否零值给出，半截会重建出一个既非首版
    -- 又非修订版的东西。
    CONSTRAINT transport_charge_occurrence_revision_coupled
        CHECK (
            (corrects_version IS NULL AND revision_kind IS NULL
             AND revision_basis IS NULL AND revised_at IS NULL)
            OR (corrects_version IS NOT NULL AND revision_kind IS NOT NULL
                AND revision_basis IS NOT NULL AND revised_at IS NOT NULL)
        ),

    -- 修订走向封闭二值。范围更正需要自己的形状、另票落地——领域刻意没有第三格，库面同。
    CONSTRAINT transport_charge_occurrence_revision_kind_closed
        CHECK (revision_kind IS NULL OR revision_kind IN ('INVALIDATED', 'SUPERSEDED')),

    CONSTRAINT transport_charge_occurrence_revision_basis_not_blank
        CHECK (revision_basis IS NULL OR btrim(revision_basis) <> ''),

    -- 沿用原版本号就是覆盖，不是修订。
    CONSTRAINT transport_charge_occurrence_revision_not_self
        CHECK (corrects_version IS NULL OR corrects_version <> validity_version),

    CONSTRAINT transport_charge_occurrence_revised_after_occurred
        CHECK (revised_at IS NULL OR revised_at >= occurred_at)
);

-- 发生项的对象范围。逐对象一行：CONTEXT 要求每条发生项固定其成员，而成员差异不能被
-- 整条发生项的结果覆盖。
CREATE TABLE transport_fulfillment.transport_charge_occurrence_member (
    tenant_id        text NOT NULL,
    occurrence_ref   text NOT NULL,
    validity_version text NOT NULL,
    object_ref       text NOT NULL,

    CONSTRAINT transport_charge_occurrence_member_pkey
        PRIMARY KEY (tenant_id, occurrence_ref, validity_version, object_ref),

    CONSTRAINT transport_charge_occurrence_member_parent_fkey
        FOREIGN KEY (tenant_id, occurrence_ref, validity_version)
        REFERENCES transport_fulfillment.transport_charge_occurrence
            (tenant_id, occurrence_ref, validity_version),

    CONSTRAINT transport_charge_occurrence_member_not_blank
        CHECK (btrim(object_ref) <> '')
);

-- 一条**故意没有下沉**的约束：成员非空（`len(spec.Members) == 0` 即拒）。它是跨表条件，
-- CHECK 表达不动；靠外键也守不住——外键管的是成员指向存在的父行，不是父行必须有成员。
-- 它留在领域（构造门与重建门各守一次），写在这里是因为读到上面那张空表定义的人会问一句。
