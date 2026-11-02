-- 异常案件与客户索赔项的单据号（ADR-0355 决定二、三、七、八；CONTEXT「异常案件」「客户索赔项」）。
--
-- 每张表加两格，都可空：document_number 是开立事务里取到的号；numberless_reason 只在开立时单据编号时区未配置、单据照常
-- 开立而没带号时落值。两格都空就是本迁移之前开立的旧行——「上线前开立」不另落一个值，旧行一行不改、不回填（决定三：
-- 不补号），本迁移因此在有数据的库上照样施加。
--
-- （租户，单据号）唯一是最后一道保险（决定八）：取号编排先查已用、撞了跳号，正常路径撞不上这把键；它挡的是绕过编排的
-- 写入。没有号的行（NULL）不参与唯一。号的格式归规则册，这里只拒空白，不复判模板。

ALTER TABLE visibility_exception.exception_case
    ADD COLUMN document_number text,
    ADD COLUMN numberless_reason text;

ALTER TABLE visibility_exception.exception_case
    ADD CONSTRAINT exception_case_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    -- 无号原因只有一个可落的值：有号的行不带原因，「上线前开立」两格都空。
    ADD CONSTRAINT exception_case_numberless_reason_closed_set
        CHECK (
            numberless_reason IS NULL
            OR (numberless_reason = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' AND document_number IS NULL)
        ),
    ADD CONSTRAINT exception_case_document_number_unique
        UNIQUE (tenant_id, document_number);

ALTER TABLE visibility_exception.claim_item
    ADD COLUMN document_number text,
    ADD COLUMN numberless_reason text;

ALTER TABLE visibility_exception.claim_item
    ADD CONSTRAINT claim_item_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT claim_item_numberless_reason_closed_set
        CHECK (
            numberless_reason IS NULL
            OR (numberless_reason = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' AND document_number IS NULL)
        ),
    ADD CONSTRAINT claim_item_document_number_unique
        UNIQUE (tenant_id, document_number);
