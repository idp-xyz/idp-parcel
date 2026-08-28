-- 产品—渠道映射登记册（CONTEXT「产品—渠道映射」+ 票 admin-remainder-mechanism-batch/02）：
-- 服务产品版本 × 渠道产品标识引用的版本化登记。
--
-- 键=租户+映射标识+修订，修订从 1 起连续递增；调整绑定或区间形成新修订行，绝不
-- UPDATE（ADR-0031 登记面纪律，同 0015 四册）。产品以（对象标识+版本号）引用，版本
-- 正文在 commercial_version 上，本表不抄第二份；册间不设外键——登记册各自只增，
-- 悬空引用由写入用例把门（同 0015 legal_entity_registration 的裁决）。
--
-- channel_refs 是渠道绑定格：字符串数组。空数组 = 显式登记的“未配置”（domain
-- .UnconfiguredChannelBinding 是唯一进得来的路——零值绑定过不了登记信封的门），
-- 非空 = 已配置的渠道产品标识引用。渠道本体不在本上下文预造（ADR-0072 否决预拟
-- 渠道表），因此没有渠道表可供外键。
--
-- 不存状态列：映射没有独立状态代数（CONTEXT 生命周期节），是否参与新的渠道决策由
-- 有效区间对时点导出，存状态列等于存一份会过期的推导。

CREATE TABLE party_commercial.product_channel_mapping_registration (
    tenant_id             text        NOT NULL,
    mapping_id            text        NOT NULL,
    revision              integer     NOT NULL,

    product_object_id     text        NOT NULL,
    product_version_label text        NOT NULL,
    channel_refs          jsonb       NOT NULL,
    basis_ref             text        NOT NULL,
    effective_starts_at   timestamptz NOT NULL,
    effective_ends_at     timestamptz,
    content_digest        text        NOT NULL,
    snapshot              jsonb       NOT NULL,
    recorded_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT product_channel_mapping_registration_pkey
        PRIMARY KEY (tenant_id, mapping_id, revision),

    CONSTRAINT product_channel_mapping_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(mapping_id) <> ''
            AND btrim(product_object_id) <> ''
            AND btrim(product_version_label) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT product_channel_mapping_registration_revision_positive
        CHECK (revision >= 1),

    -- 绑定格必须是数组；数组内逐个引用非空白由领域构造门把守，SQL 只锁形状。
    CONSTRAINT product_channel_mapping_registration_channels_array
        CHECK (jsonb_typeof(channel_refs) = 'array'),

    CONSTRAINT product_channel_mapping_registration_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at),

    CONSTRAINT product_channel_mapping_registration_snapshot_object
        CHECK (snapshot IS NOT NULL AND jsonb_typeof(snapshot) = 'object')
);

-- 目录按（租户+登记时间倒序）上列；最新修订经主键即可定位，不另建索引（同 0015）。
CREATE INDEX product_channel_mapping_registration_by_tenant
    ON party_commercial.product_channel_mapping_registration
        (tenant_id, recorded_at DESC);
