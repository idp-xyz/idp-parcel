-- 供应商预期成本：内部预期金额的版本册（UC-SA-002 形成，UC-SA-004 逐行匹配按版本
-- 引用）。
--
-- 列上没有账单主张、审核应付或付款字段——「内部预期不冒充供应商主张」在表上与在
-- 类型上同样是结构性的（CONTEXT 供应商成本与共享分摊）。
--
-- 主键取（租户+版本）：计价纠错换版本、原版本保留，一份成本的历史因此是多行而不是
-- 一行被改写。首版唯一由部分唯一索引承担：幂等三维「同一发生项、费用项目和规则版本
-- 不得重复形成预期成本」说的是首版，纠错版本共用同一三维、只能追加。
--
-- 可空列上的 CHECK 先验 IS NULL / IS NOT NULL 再取值，过 SQL 三值逻辑那一眼
-- （f822e1d 入册的纪律）。

CREATE TABLE settlement_accounting.supplier_expected_cost (
    tenant_id             text        NOT NULL,
    version               text        NOT NULL,

    occurrence_id         text        NOT NULL,
    occurrence_reason     text        NOT NULL,
    occurrence_version    text        NOT NULL,
    occurred_at           timestamptz NOT NULL,
    fee_item              text        NOT NULL,
    purchase_rule_version text        NOT NULL,
    agreement_ref         text        NOT NULL,
    evaluation_ref        text        NOT NULL,
    original_currency     text        NOT NULL,
    original_minor        bigint      NOT NULL,
    settlement_currency   text        NOT NULL,
    settlement_minor      bigint      NOT NULL,
    conversion_ref        text,
    prior_version         text,
    correction_reason     text,
    recorded_at           timestamptz NOT NULL,
    inserted_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT supplier_expected_cost_pkey
        PRIMARY KEY (tenant_id, version),

    CONSTRAINT supplier_expected_cost_identity_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(version) <> ''
            AND btrim(occurrence_id) <> ''
            AND btrim(occurrence_reason) <> ''
            AND btrim(occurrence_version) <> ''
            AND btrim(fee_item) <> ''
            AND btrim(purchase_rule_version) <> ''
            AND btrim(agreement_ref) <> ''
            AND btrim(evaluation_ref) <> ''
            AND btrim(original_currency) <> ''
            AND btrim(settlement_currency) <> ''
        ),

    CONSTRAINT supplier_expected_cost_amounts_positive
        CHECK (original_minor > 0 AND settlement_minor > 0),

    -- 跨币种必带评价内换算步骤：缺它的第二个金额只能是自行取汇率补算出来的，而
    -- 换算依据归 parcel-pricing 在评价内完成（AT-SA-176/177）。
    CONSTRAINT supplier_expected_cost_conversion_present
        CHECK (
            original_currency = settlement_currency
            OR (conversion_ref IS NOT NULL AND btrim(conversion_ref) <> '')
        ),

    -- 同币种两额必须相等，但只对首版：没有换算却出现两个数，只可能是自行取汇率补算
    -- 出来的。纠错版本不在此列——计价纠错重述的是结算金额、原币金额原样留着，同币种
    -- 的纠错因此本就可以两额不等（AppendCorrection 正是这样产生它的）。
    CONSTRAINT supplier_expected_cost_same_currency_amounts_agree
        CHECK (
            prior_version IS NOT NULL
            OR original_currency <> settlement_currency
            OR original_minor = settlement_minor
        ),

    -- 纠错两件成对且不自指：只带回指或只带原因的行说不清它纠正的是哪一版。
    CONSTRAINT supplier_expected_cost_correction_paired
        CHECK (
            (prior_version IS NULL AND correction_reason IS NULL)
            OR (
                prior_version IS NOT NULL
                AND correction_reason IS NOT NULL
                AND btrim(prior_version) <> ''
                AND btrim(correction_reason) <> ''
                AND prior_version <> version
            )
        )
);

-- 幂等三维只约束首版；纠错版本带回指，落在索引条件之外。
CREATE UNIQUE INDEX supplier_expected_cost_first_version_unique
    ON settlement_accounting.supplier_expected_cost
       (tenant_id, occurrence_id, fee_item, purchase_rule_version)
 WHERE prior_version IS NULL;
