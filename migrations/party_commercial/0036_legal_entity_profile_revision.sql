-- 法人资料修订链（ADR-0145 决定三、五、六；CONTEXT Lifecycles「法人资料」；票 legal-entity-profile/03）：责任法人
-- 对外开票、签约与通知所用的资料——注册地址（带国家 / 地区）、税务登记号、开票资料（首版只含开票抬头）与联系人。
--
-- 键=租户+责任法人+修订，修订从 1 起连续递增；新修订是新行，绝不 UPDATE（ADR-0031 登记面纪律，同 0015 身份册）。
-- 不存状态列，也不存「当前修订」：哪一笔在某个时点有效由生效时点与修订号对时点导出
-- （domain.ResolveLegalEntityProfile），存一格就是存一份会被追溯修订改写的推导结果。开立方固定的修订引用
-- 就是本表的主键，所以行一经写下不再改动。
--
-- 不对身份登记表建外键：那张表的键带修订号，没有「一个法人一行」可引；法人在册、未停用、地址国家对得上身份
-- 由写入用例把门，判据同 0015 各表之间的引用。币种不在本表（ADR-0145 决定四）。
--
-- 结构化列供 CHECK 与人工排查，snapshot 是重建领域对象的来源、content_digest 判重放；三者由同一次写入落下。
-- 全部 CHECK 过 SQL 三值逻辑那一眼（同 0015）：可空列使用前先 IS NULL / IS NOT NULL。

CREATE TABLE party_commercial.legal_entity_profile_revision (
    tenant_id                text        NOT NULL,
    legal_entity_id          text        NOT NULL,
    revision                 integer     NOT NULL,

    basis_ref                text        NOT NULL,
    effective_from           timestamptz NOT NULL,
    address_country          text        NOT NULL,
    address_lines            jsonb       NOT NULL,
    tax_registration_numbers jsonb       NOT NULL,
    invoice_title            text,
    contacts                 jsonb       NOT NULL,
    content_digest           text        NOT NULL,
    snapshot                 jsonb       NOT NULL,
    recorded_at              timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT legal_entity_profile_revision_pkey
        PRIMARY KEY (tenant_id, legal_entity_id, revision),

    CONSTRAINT legal_entity_profile_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity_id) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT legal_entity_profile_revision_revision_positive
        CHECK (revision >= 1),

    -- 镜像 domain.RegistrationCountryCode 的形状门：地址国家要与身份上的注册国家 / 地区逐字比对。
    CONSTRAINT legal_entity_profile_revision_address_country_shape
        CHECK (address_country ~ '^[A-Z]{2}$'),

    CONSTRAINT legal_entity_profile_revision_address_lines_shape
        CHECK (jsonb_typeof(address_lines) = 'array' AND jsonb_array_length(address_lines) > 0),

    -- 税务登记号可后办、联系人可空：两格是数组即可，允许空数组。
    CONSTRAINT legal_entity_profile_revision_collections_shape
        CHECK (
            jsonb_typeof(tax_registration_numbers) = 'array'
            AND jsonb_typeof(contacts) = 'array'
        ),

    -- 开票资料可以缺（登记时不拦，开立时答资料不全，ADR-0145 决定六）；带了就不能是空白抬头。
    CONSTRAINT legal_entity_profile_revision_invoice_title_not_blank
        CHECK (invoice_title IS NULL OR btrim(invoice_title) <> ''),

    CONSTRAINT legal_entity_profile_revision_snapshot_object
        CHECK (jsonb_typeof(snapshot) = 'object')
);
