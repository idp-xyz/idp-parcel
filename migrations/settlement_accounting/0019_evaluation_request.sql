-- 评价请求登记册：UC-SA-002 步 2「请求评价」留在本上下文的登记（票 sa-cc/08）。一行 = 本上下文向
-- parcel-pricing 提交了什么结算提交主要范围、什么计算目的、哪三件合格来源引用（TF 运输收费发生项、
-- 费用项目、供应商协议版本），以及谁在何时请求。
--
-- 列上没有金额、币种、换算或评价结果——它们整组归评价（ADR-0107 / ADR-0013），请求只记「问了什么」。
-- 三件引用是调用方交进来的引用：表上不引用 TF / PC 的任何表，本上下文不从它们的内容推。
--
-- 主键取（租户+铸造 ID）（裁决 2）：后续信封与预期成本形成的消费者引的是它，不引自然键——自然键的
-- 成分一变引用就断，铸造出来的 ID 不随成分变。
--
-- 自然键唯一约束（租户+主要范围+计算目的+合格来源引用集合摘要）守幂等（裁决 2）：同自然键的第二次
-- 提交在库上撞这一条，适配器据以答`已存在`并让编排交回先到者的 ID；成分不同就是另一份请求、另一个
-- ID。摘要由领域按排序后的引用集合算出并自带规范化版本前缀（ADR-0014），库只比字节、不解释它。
--
-- 计算目的封闭词表照领域 CalculationPurpose：首发只有 BUY 供应商成本一格。词表由领域拥有，这里的
-- CHECK 是第二道拦——库里出现本上下文不认识的目的只可能是适配器或迁移的 bug。

CREATE TABLE settlement_accounting.evaluation_request (
    tenant_id          text        NOT NULL,
    request_id         text        NOT NULL,

    scope_ref          text        NOT NULL,
    purpose            text        NOT NULL,
    occurrence_id      text        NOT NULL,
    occurrence_reason  text        NOT NULL,
    occurrence_version text        NOT NULL,
    occurred_at        timestamptz NOT NULL,
    fee_item           text        NOT NULL,
    agreement_ref      text        NOT NULL,
    source_digest      text        NOT NULL,
    requested_at       timestamptz NOT NULL,
    requested_by       text        NOT NULL,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT evaluation_request_pkey
        PRIMARY KEY (tenant_id, request_id),

    CONSTRAINT evaluation_request_natural_key_unique
        UNIQUE (tenant_id, scope_ref, purpose, source_digest),

    CONSTRAINT evaluation_request_identity_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(request_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(purpose) <> ''
            AND btrim(occurrence_id) <> ''
            AND btrim(occurrence_reason) <> ''
            AND btrim(occurrence_version) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(agreement_ref) <> ''
            AND btrim(source_digest) <> ''
            AND btrim(requested_by) <> ''
        ),

    CONSTRAINT evaluation_request_purpose_closed
        CHECK (purpose IN ('BUY_SUPPLIER_COST'))
);
