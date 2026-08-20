-- 同币种两额必须相等，对所有版本成立（ADR-0067 决定四）：没有换算步骤，第二个数
-- 就没有依据，只可能是自行取汇率补算出来的——这条理由在纠错版本上一字不变。
--
-- 本文件取代 0008 的 supplier_expected_cost_same_currency_amounts_agree：那一版带
-- `prior_version IS NOT NULL` 的例外支，因为当时 AppendCorrection 只重述结算金额、
-- 原币金额停在旧评价上，同币种纠错版本两额本就可以不等。ADR-0067 裁定计价纠错整组
-- 重述一个评价的计价结果（原币金额随新评价重述、与新结算金额相等），例外随之作废；
-- 0008 里那条 CHECK 的注释自此与现行口径相反，以本文件为准。已施加的迁移不可改写
-- （校验和把关），故新文件重建约束（先例：pilot_governance 0002、visibility_exception
-- 0003）；表此刻无真实数据，直接换无需清洗。

ALTER TABLE settlement_accounting.supplier_expected_cost
    DROP CONSTRAINT supplier_expected_cost_same_currency_amounts_agree,
    ADD CONSTRAINT supplier_expected_cost_same_currency_amounts_agree
        CHECK (
            original_currency <> settlement_currency
            OR original_minor = settlement_minor
        );
