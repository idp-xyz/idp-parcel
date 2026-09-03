-- 价格政策口径册（票 party-commercial-context-gaps/02、06）：一行记一个价格规则版本声明的
-- 计价口径——税务口径、体积口径，以及可缺的汇率口径（CONTEXT「它还声明计价所需的商业口径」）。
--
-- 与价格政策正文册（0010）分表而不是往它上面加列：CommercialPricePolicy 表达的是**选用时要观察
-- 的正文**（方向、方案、范围、区间），口径不参与选用（ResolveCommercialPricePolicy 按方向 + 范围 +
-- 区间选）。按票 03 定下的判据，不参与选择的正文不进整册、不进 ViewRevision——口径走族 B：按版本
-- 点读（ports.PricePolicyCaliberView），与信用政策册（0020）、供应商协议册（0021）同一条路。
-- 本表因此不进 LoadForScope。
--
-- 外键钉在**政策正文行**上而不是版本册上，且带 direction：口径只能挂在同方向的政策正文行上。
-- 体积口径按方向劈开（销售方向必须声明系数、采购与法人间禁止），一份 SELL 的口径挂到 BUY 的
-- 政策上，等于给采购方向声明了承运商价卡之外的第二份系数——领域 ConsistentWithDirection 在
-- 发布编排里核一遍，这里是库上的镜像。为此先在 0010 的表上补一条含 direction 的唯一约束作外键
-- 落点（主键已唯一，多一列仍唯一），不改写 0010 自身。连带地，没有正文行就登不了口径：
-- 「只给口径不给正文」在库上就写不出来。
--
-- 三处「缺席是真话」都由 CHECK 钉住，与领域构造门同判据：
--   * tax_classification_ref 与 tax_disposition 充要耦合：含税/未税恰要求一份分类，不适用恰要求
--     没有（domain.NewTaxCaliber）；
--   * volumetric_factor_ref 与 direction 充要耦合：SELL 必有、BUY/INTERNAL 必无
--     （domain.NewVolumetricCaliber）；
--   * 汇率三列全有或全无（domain.NewFxCaliber 两格缺一不可）：全无是「没声明汇率口径」这句合法
--     缺席，半缺是缺件，进不来。
--
-- 加点规则不在本表，且不是漏掉：票 02 裁 (a) 首发显式未决，重启条件记在票面。
--
-- object_kind CHECK = 6（PriceRuleObject）。行属实例半边：真实牌价类型、税务分类、体积系数
-- 由租户登记，今天没有租户因而本表为空。

ALTER TABLE party_commercial.commercial_price_policy
    ADD CONSTRAINT commercial_price_policy_version_direction_key
        UNIQUE (tenant_id, object_kind, object_id, version_label, direction);

CREATE TABLE party_commercial.price_policy_caliber (
    tenant_id               text        NOT NULL,
    object_kind             smallint    NOT NULL,
    object_id               text        NOT NULL,
    version_label           text        NOT NULL,
    direction               text        NOT NULL,

    tax_disposition         text        NOT NULL,
    tax_classification_ref  text,
    volumetric_factor_ref   text,
    fx_quote_type_ref       text,
    fx_as_of_semantics_ref  text,
    fx_as_of_policy_version text,
    registered_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT price_policy_caliber_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT price_policy_caliber_policy_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label, direction)
        REFERENCES party_commercial.commercial_price_policy
            (tenant_id, object_kind, object_id, version_label, direction),

    CONSTRAINT price_policy_caliber_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND (tax_classification_ref IS NULL OR btrim(tax_classification_ref) <> '')
            AND (volumetric_factor_ref IS NULL OR btrim(volumetric_factor_ref) <> '')
            AND (fx_quote_type_ref IS NULL OR btrim(fx_quote_type_ref) <> '')
            AND (fx_as_of_semantics_ref IS NULL OR btrim(fx_as_of_semantics_ref) <> '')
            AND (fx_as_of_policy_version IS NULL OR btrim(fx_as_of_policy_version) <> '')
        ),

    CONSTRAINT price_policy_caliber_price_rule_only
        CHECK (object_kind = 6),

    CONSTRAINT price_policy_caliber_direction_closed
        CHECK (direction IN ('BUY', 'SELL', 'INTERNAL')),

    CONSTRAINT price_policy_caliber_tax_disposition_closed
        CHECK (tax_disposition IN ('TAX_INCLUSIVE', 'TAX_EXCLUSIVE', 'TAX_NOT_APPLICABLE')),

    CONSTRAINT price_policy_caliber_tax_classification_coupled
        CHECK ((tax_disposition <> 'TAX_NOT_APPLICABLE') = (tax_classification_ref IS NOT NULL)),

    CONSTRAINT price_policy_caliber_volumetric_factor_coupled
        CHECK ((direction = 'SELL') = (volumetric_factor_ref IS NOT NULL)),

    CONSTRAINT price_policy_caliber_fx_all_or_none
        CHECK (
            (fx_quote_type_ref IS NULL
                AND fx_as_of_semantics_ref IS NULL
                AND fx_as_of_policy_version IS NULL)
            OR (fx_quote_type_ref IS NOT NULL
                AND fx_as_of_semantics_ref IS NOT NULL
                AND fx_as_of_policy_version IS NOT NULL)
        )
);
