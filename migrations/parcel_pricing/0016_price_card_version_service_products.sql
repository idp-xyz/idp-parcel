-- 售价卡的适用服务产品列（ADR-0378 决定四；票 declared-goods-lines/14）。
--
-- SELL·客户费用价卡可以声明一组它计价的服务产品（参与方与商业的服务产品对象标识，不带版本）；选卡按委托申报的服务产品
-- 分两层，先看指名这个产品的卡、再看不声明产品的卡。产品是方案内容、权威在 snapshot，这一列与 scope、direction、purpose
-- 那几列同一个身份：给选卡的谓词用，读回时与快照核对，列与快照分岔即行被改过。
--
-- 空数组即不声明，接其余产品——这正是本列出现之前每张卡的原义，所以既有各行取缺省 '{}' 而不回填任何东西。数组按标识
-- 升序、无重复，与快照同序：领域保存时就排好了，适配器原样写入。

-- 只有 SELL·客户费用的卡能声明（ADR-0378 决定一），领域构造门与读回门都守；这里再守一道，绕过领域直接写库的行同样存不下。

ALTER TABLE parcel_pricing.price_card_version
    ADD COLUMN service_products text[] NOT NULL DEFAULT '{}',
    ADD CONSTRAINT price_card_version_service_products_sell_only
        CHECK (cardinality(service_products) = 0 OR (direction = 'SELL' AND purpose = 'CUSTOMER_CHARGE'));
