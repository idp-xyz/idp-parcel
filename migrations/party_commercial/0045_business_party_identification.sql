-- 业务参与方身份登记加识别信息（票 business-party-identification/02；ADR-0367 决定一、三；PC CONTEXT Rules
-- 「业务参与方身份登记可带注册国家 / 地区与终身注册号……」「业务参与方身份可带外部系统编码……」）。
--
-- 只加格、不改已施加的 0015：registration_country 与 lifetime_registration_numbers 与法人册 0034 同形（国家 ISO 3166
-- 两位大写字母形状，号数组每项 {typeCode, number}）；identity_correction_basis 只随改了或去掉已登的号的更正修订出现；
-- external_references 是外部系统编码数组，每项 {system, purpose, code}。号是否属该国身份层类型、格式是否合格、同号
-- 同码有没有别人持有，由写入用例判，库只钉单看一行就判得出的形状。
--
-- 四格都可空：往来单位的识别信息本来可缺，本格落地之前的存量行四格全空，新 CHECK 对它们恒真。与 0034 的一处不同：
-- 更正依据可以脱离号单独出现——去掉一个录错的号也是更正——所以不钉「更正依据要有国家」，只钉非空白与不落在修订 1。

ALTER TABLE party_commercial.business_party_registration
    ADD COLUMN registration_country          text,
    ADD COLUMN lifetime_registration_numbers jsonb,
    ADD COLUMN identity_correction_basis     text,
    ADD COLUMN external_references           jsonb;

ALTER TABLE party_commercial.business_party_registration
    ADD CONSTRAINT business_party_registration_identity_paired
        CHECK ((registration_country IS NULL) = (lifetime_registration_numbers IS NULL)),
    ADD CONSTRAINT business_party_registration_country_shape
        CHECK (registration_country IS NULL OR registration_country ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT business_party_registration_lifetime_numbers_shape
        CHECK (
            lifetime_registration_numbers IS NULL
            OR (jsonb_typeof(lifetime_registration_numbers) = 'array'
                AND jsonb_array_length(lifetime_registration_numbers) > 0)
        ),
    ADD CONSTRAINT business_party_registration_correction_shape
        CHECK (identity_correction_basis IS NULL OR (btrim(identity_correction_basis) <> '' AND revision > 1)),
    ADD CONSTRAINT business_party_registration_external_references_shape
        CHECK (
            external_references IS NULL
            OR (jsonb_typeof(external_references) = 'array' AND jsonb_array_length(external_references) > 0)
        );

-- 查重按号与按编码找持有方，走 jsonb 包含；两张表都不大，索引只为不让每次登记都整表扫。
CREATE INDEX business_party_registration_lifetime_numbers_idx
    ON party_commercial.business_party_registration USING gin (lifetime_registration_numbers jsonb_path_ops);
CREATE INDEX business_party_registration_external_references_idx
    ON party_commercial.business_party_registration USING gin (external_references jsonb_path_ops);
