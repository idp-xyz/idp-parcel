-- 计价参考序列种类扩一格：按期公布的金额（ADR-0110 Decision 一；票 price-card-shape-gaps/02）。
--
-- 0003 把 kind 的封闭集钉在库层（FUEL_RATE / EXCHANGE_RATE）；领域封闭集加了 PUBLISHED_AMOUNT，库层的那一组
-- 词跟着扩，仍与 domain.ReferenceSeriesKind 逐字同组。只换 CHECK，不动任何既有行——序列行只增不改。
--
-- 金额序列的币种在快照里（PRS-2 内 omitempty 新字段），列面不另设：列面只承担键、比对与包络检索，权威内容
-- 在快照。金额序列不要求口径引用（fx_demands_quote_basis 只钉 EXCHANGE_RATE，照旧）。

ALTER TABLE parcel_pricing.reference_series_version
    DROP CONSTRAINT reference_series_version_kind_closed;

ALTER TABLE parcel_pricing.reference_series_version
    ADD CONSTRAINT reference_series_version_kind_closed
        CHECK (kind IN ('FUEL_RATE', 'EXCHANGE_RATE', 'PUBLISHED_AMOUNT'));
