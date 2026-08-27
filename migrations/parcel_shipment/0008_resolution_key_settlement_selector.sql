-- 解析键登记面扩结算三维，放行 SETTLEMENT_POLICY（ADR-0080）。
--
-- 0007 的 `..._bases_closed` 把 SETTLEMENT_POLICY 与 PRICE_RULE 一起挡在名集之外，
-- 理由是「这两类必需依据要求解析键额外携带结算选择器/价格方向，本登记面没有那些维度」，
-- 并留了一句「日后接通结算作用域缝时随新决定扩列」。这就是那次扩列。
--
-- **只放行 SETTLEMENT_POLICY，PRICE_RULE 仍挡着。** 价格方向那一维与结算三维不是同一件
-- 事，且价格政策的快照重建至今缺席（见 domain.RehydrateAdoptedBasisSpec 的注释）——它是
-- 另一道决定，不在这里预留。
--
-- **只有三维，没有合同维。** ADR-0044 的结算选择器是四维（相对方、客户合同版本、费用
-- 范围、币种），但合同那一维在闭包里是**结论**：同一个闭包的另一项必需依据要解析的正是
-- 「哪一版合同适用」，由 ResolveCommercialClosure 解出后填（ADR-0080）。登记进来就是消费
-- 方在指定该选中哪个商业版本（`psports.CommercialBasisQuery` 明禁的那件事），而且租户每换
-- 一版合同都得有人手工改这一行——漏改不报错，只是解析不出结果，看起来像「没登记过结算
-- 政策」。
--
-- 三维在租户配置里是稳的：结算相对方、费用范围与币种由商业约定给定，不随合同换版而变。
--
-- 迁移不回改 0007：既有约束在这里丢弃重建，而不是就地改写那份文件。

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD COLUMN settlement_counterparty_ref text,
    ADD COLUMN settlement_charge_scope_ref text,
    ADD COLUMN settlement_currency_code    text;

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    DROP CONSTRAINT commercial_resolution_key_registration_bases_closed;

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD CONSTRAINT commercial_resolution_key_registration_bases_closed
        CHECK (required_bases <@ ARRAY[
            'SERVICE_PRODUCT',
            'CUSTOMER_CONTRACT',
            'SUPPLIER_AGREEMENT',
            'ACCEPTANCE_RULE_PACKAGE',
            'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
            'SETTLEMENT_POLICY',
            'CREDIT_POLICY',
            'AUTHORIZATION_RULE'
        ]::text[]);

-- 三维与结算依据同进同出，且要结算就必须一并要客户合同。
--
-- 写成一条整体约束而不是三条各管一维：部分给出的选择器与「给了维度却没要结算依据」都必须
-- 挡在这里，逐维分写会让「相对方在场、费用范围缺席、也没要结算依据」这类组合从缝里过去。
--
-- 末一条（要结算就要合同）不是附赠的严格：合同维要由闭包解出的合同来填，不请求合同就永远
-- 没人填得上——那是一个立不起来的键，放它进来只会在后面变成一句像是「没有适用结算政策」
-- 的话。领域侧 ClosureResolutionKey.minimumIdentityEstablished 是同一判据的另一道镜像。
ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD CONSTRAINT commercial_resolution_key_registration_settlement_paired
        CHECK (
            (
                settlement_counterparty_ref IS NOT NULL
                AND settlement_charge_scope_ref IS NOT NULL
                AND settlement_currency_code IS NOT NULL
                AND 'SETTLEMENT_POLICY' = ANY (required_bases)
                AND 'CUSTOMER_CONTRACT' = ANY (required_bases)
            )
            OR (
                settlement_counterparty_ref IS NULL
                AND settlement_charge_scope_ref IS NULL
                AND settlement_currency_code IS NULL
                AND NOT ('SETTLEMENT_POLICY' = ANY (required_bases))
            )
        );

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD CONSTRAINT commercial_resolution_key_registration_settlement_not_blank
        CHECK (
            (settlement_counterparty_ref IS NULL OR btrim(settlement_counterparty_ref) <> '')
            AND (settlement_charge_scope_ref IS NULL OR btrim(settlement_charge_scope_ref) <> '')
            AND (settlement_currency_code IS NULL OR btrim(settlement_currency_code) <> '')
        );
