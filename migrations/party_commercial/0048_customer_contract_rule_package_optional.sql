-- 票 declared-goods-lines/11（ADR-0376 决定三）：客户合同正文的接单规则包一格改为可缺。接单规则包随服务产品选——规则包版本
-- 指名它的服务产品，解析按委托申报的产品收窄（ADR-0153 决定四）——一个货主账户下几个服务产品各配一份规则包时，合同不该再
-- 指名其中某一份。0012 头注「rule_package_id 必存」的那条理由随之不成立：NewCustomerContract 不再要求规则包引用。
--
-- 可缺而不可空白：在场的值照旧过非空白核对，缺就是 NULL，不拿空串当「没指名」——空串与缺席在读面上是两种说法。已登的行
-- 一行不动。

ALTER TABLE party_commercial.customer_contract_content
    ALTER COLUMN rule_package_id DROP NOT NULL;

ALTER TABLE party_commercial.customer_contract_content
    DROP CONSTRAINT customer_contract_content_not_blank,
    ADD CONSTRAINT customer_contract_content_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND (rule_package_id IS NULL OR btrim(rule_package_id) <> '')
        );
