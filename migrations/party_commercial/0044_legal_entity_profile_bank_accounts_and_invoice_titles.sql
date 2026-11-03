-- 法人资料修订加三格（ADR-0359 决定三、四；CONTEXT Rules「法人资料」）：收款账户、多语言开票抬头与主抬头的语言。
--
-- 不改已施加的 0036：ALTER TABLE 加列，既有行三列皆 NULL，读回即「这一版不带收款账户 / 只有主抬头」，不回填。
-- 三列都是镜像列，供 CHECK 与人工排查；snapshot 仍是重建领域对象的唯一来源，含这三格的快照写 shape LEP-4
-- （domain 的构造门与 postgres 适配器的 checkShape 把守形状与内容相符）。
--
-- 收款账户属 party-commercial：对账单、发票要印的是法人自己的银行账户，不是 settlement-accounting 的结算账户。
-- 账户的币种是账户自身的属性，不是结算币种的决定（ADR-0145 决定四不变）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（同 0015）：可空列使用前先 IS NULL / IS NOT NULL。

ALTER TABLE party_commercial.legal_entity_profile_revision
    ADD COLUMN bank_accounts          jsonb,
    ADD COLUMN invoice_titles         jsonb,
    ADD COLUMN invoice_title_language text;

ALTER TABLE party_commercial.legal_entity_profile_revision
    -- 带了就是非空数组：缺席写 NULL，空数组没有第二种「缺席」的写法。
    ADD CONSTRAINT legal_entity_profile_revision_bank_accounts_shape
        CHECK (
            bank_accounts IS NULL
            OR (jsonb_typeof(bank_accounts) = 'array' AND jsonb_array_length(bank_accounts) > 0)
        ),

    ADD CONSTRAINT legal_entity_profile_revision_invoice_titles_shape
        CHECK (
            invoice_titles IS NULL
            OR (jsonb_typeof(invoice_titles) = 'array' AND jsonb_array_length(invoice_titles) > 0)
        ),

    ADD CONSTRAINT legal_entity_profile_revision_invoice_title_language_not_blank
        CHECK (invoice_title_language IS NULL OR btrim(invoice_title_language) <> ''),

    -- 多语言抬头与主抬头语言挂在主抬头上：没有主抬头就没有开票资料，这两格也无处可挂；`资料不全`仍只看主抬头。
    ADD CONSTRAINT legal_entity_profile_revision_localized_titles_need_main_title
        CHECK (
            invoice_title IS NOT NULL
            OR (invoice_titles IS NULL AND invoice_title_language IS NULL)
        );
