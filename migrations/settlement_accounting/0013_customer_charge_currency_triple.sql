-- 客户费用补齐币种三件组：原币金额、合同结算币金额与换算依据（SA CONTEXT「每条费用
-- 分别保存原币金额、合同结算币金额及换算依据」，三件从同一个评价采用来的一组）。
-- 客户费用受三件组约束由票 supplier-expected-cost-correction/04 裁定：CONTEXT 字面
-- 即权威，文档不动、代码与库面补齐。
--
-- 0003 已施加不可改写（校验和把关），本文件以改列补列落地。既有单币种一对列改名为
-- 结算币一对——rename 不改约束语义，既有 CHECK（正数、非空白、确认成对）随列走；再
-- 补原币一对与换算依据列。既有行全部由单币种口径写下（当时的形成门只收一对金额），
-- 唯一合法解读是原币即结算币、两额相等、无换算步骤，据此回填后钉 NOT NULL。表此刻
-- 无生产数据（产品尚无租户），回填只为不假设环境库为空。
--
-- 同币种两额相等与跨币种必备换算步骤两条 CHECK 与供应商侧同款（0008 立、0012 撤销
-- 例外支后的形状）：领域不变量在库内再守一遍。

ALTER TABLE settlement_accounting.customer_charge
    RENAME COLUMN currency TO settlement_currency;
ALTER TABLE settlement_accounting.customer_charge
    RENAME COLUMN amount_minor TO settlement_minor;

ALTER TABLE settlement_accounting.customer_charge
    ADD COLUMN original_currency text,
    ADD COLUMN original_minor    bigint,
    ADD COLUMN conversion_ref    text;

UPDATE settlement_accounting.customer_charge
   SET original_currency = settlement_currency,
       original_minor    = settlement_minor
 WHERE original_currency IS NULL;

ALTER TABLE settlement_accounting.customer_charge
    ALTER COLUMN original_currency SET NOT NULL,
    ALTER COLUMN original_minor    SET NOT NULL;

ALTER TABLE settlement_accounting.customer_charge
    ADD CONSTRAINT customer_charge_original_currency_not_blank
        CHECK (btrim(original_currency) <> ''),

    ADD CONSTRAINT customer_charge_original_amount_positive
        CHECK (original_minor > 0),

    -- 跨币种必带评价内换算步骤：缺它的第二个金额只能是自行取汇率补算出来的，而
    -- 换算依据归 parcel-pricing 在评价内完成，本上下文只保存不重算。
    ADD CONSTRAINT customer_charge_conversion_present
        CHECK (
            original_currency = settlement_currency
            OR (conversion_ref IS NOT NULL AND btrim(conversion_ref) <> '')
        ),

    -- 同币种两额必须相等：没有换算步骤，第二个数就没有依据。
    ADD CONSTRAINT customer_charge_same_currency_amounts_agree
        CHECK (
            original_currency <> settlement_currency
            OR original_minor = settlement_minor
        );
