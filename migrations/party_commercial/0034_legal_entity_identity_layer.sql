-- 责任法人身份登记加身份层两格与身份更正依据（票 legal-entity-profile/02；ADR-0145 决定一、二；PC CONTEXT Rules
-- 「责任法人身份登记必须带注册国家 / 地区与至少一个终身注册号，缺一拒登……录错按内容更正形成新的登记修订并携带
-- 更正依据」）。
--
-- 只加格、不改已施加的 0015：registration_country 是注册国家 / 地区（ISO 3166 两位大写字母形状，码表不内置，与 0033
-- 同一判法）；lifetime_registration_numbers 是终身注册号数组，每项 {typeCode, number}，号是否属该国身份层类型、格式
-- 是否合格由写入用例按注册号类型目录判，库只钉形状；identity_correction_basis 只随改了身份层的更正修订出现。
--
-- 三格都可空：本格落地之前登记的历史修订没有身份层，读回照样成立——新登记与新修订必须带两格的门在写入用例，不在
-- 这里。库钉三件事：两格同空同有（缺一格的行不是任何一种合法修订）、国家形状与号数组非空、更正依据不脱离身份层
-- 单独出现。存量行三格全空，新 CHECK 对它们恒真；ADD CONSTRAINT 默认校验存量行，这一点由它自己证。

ALTER TABLE party_commercial.legal_entity_registration
    ADD COLUMN registration_country          text,
    ADD COLUMN lifetime_registration_numbers jsonb,
    ADD COLUMN identity_correction_basis     text;

ALTER TABLE party_commercial.legal_entity_registration
    ADD CONSTRAINT legal_entity_registration_identity_paired
        CHECK ((registration_country IS NULL) = (lifetime_registration_numbers IS NULL)),
    ADD CONSTRAINT legal_entity_registration_country_shape
        CHECK (registration_country IS NULL OR registration_country ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT legal_entity_registration_lifetime_numbers_shape
        CHECK (
            lifetime_registration_numbers IS NULL
            OR (jsonb_typeof(lifetime_registration_numbers) = 'array'
                AND jsonb_array_length(lifetime_registration_numbers) > 0)
        ),
    ADD CONSTRAINT legal_entity_registration_correction_needs_identity
        CHECK (
            identity_correction_basis IS NULL
            OR (registration_country IS NOT NULL AND btrim(identity_correction_basis) <> '')
        );
