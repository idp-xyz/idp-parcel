-- 解析键登记面放行 CUSTOMER_SERVICE_RULE（ADR-0136；票 ps-port-remainder/09）。
--
-- ADR-0136 决定三：「客户合同版本与客户服务规则版本都必须在接受时闭包的必需依据里」——索赔资格两维
-- 按目标包裹所属委托接受时固定的闭包选规则版本，租户要在本登记面把客户服务规则列进必需依据，接受时
-- 闭包才会采用它。0007 立、0008 重加的 `..._bases_closed` 白名单里没有这一类，于是「去 PS 解析键登记面
-- 把它列进必需依据」这条恢复动作在库这一道就被挡回；本迁移只在白名单里加这一格。
--
-- **只加一格，不改既有格。** PRICE_RULE 仍挡着（价格方向那一维本登记面没有，0008 头注的理由不变）；
-- 结算三维、信用两维的配对约束与「在场即非空」约束一字不动。客户服务规则没有随它同进同出的选择维度
-- ——它按范围、锚点与已采用的合同解出（ADR-0104 决定四的既有闭包路），键上不多带任何东西，所以只有
-- 白名单要改。
--
-- 要不要随合同维一并「必登」不在这里：本迁移只让它可登（票面「要裁的」1 归 PS owner）。
--
-- 迁移不回改 0007 / 0008：既有约束在这里丢弃重建，而不是就地改写已施加的那两份文件。

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
            'AUTHORIZATION_RULE',
            'CUSTOMER_SERVICE_RULE'
        ]::text[]);
