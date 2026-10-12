-- 客户合同正文指名它的货主客户账户（ADR-0177 决定一；票 customer-service-rule-contract-applicability/07）。
--
-- 闭包按解析键上的客户账户收窄客户合同候选（决定二），选法要从正文这一格读到账户；PC CONTEXT 写定「合同版本必须明确
-- 关联…货主客户账户」。不对客户账户登记表建外键：那张表的键带修订号，没有「一个账户一行」可引，判据同 0015 各表之间的
-- 引用，账户在不在册由写入用例把门。
--
-- 列可空只为本迁移之前已写下的行：它们没有这一格可补，也不该被猜一个。写入面自本迁移起恒写这一格（领域构造门要它），
-- 读到空的旧行时点读口报错、不折成任何一户的合同；演示库按新摘要 `seed.sh --reset` 重灌（ADR-0177 决定五）。
ALTER TABLE party_commercial.customer_contract_content
    ADD COLUMN customer_account_id text;

ALTER TABLE party_commercial.customer_contract_content
    ADD CONSTRAINT customer_contract_content_account_not_blank
        CHECK (customer_account_id IS NULL OR btrim(customer_account_id) <> '');
