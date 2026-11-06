-- 票 catalogue-read-pagination-rollout/04：parcel-pricing 五册迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」取页，
-- 每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。另放的标识类可排维（价卡的 planId、序列的 seriesId）由各表
-- （tenant_id, 标识, 版本）主键承担，不另建。

-- 评价登记册：缺省序落册时刻倒序，标识评价号。
CREATE INDEX evaluation_by_tenant_recorded
    ON parcel_pricing.evaluation (tenant_id, recorded_at DESC, evaluation_id DESC);

-- 价卡目录：缺省序登记时刻倒序，标识（方案、版本）。
CREATE INDEX price_card_version_by_tenant_registered
    ON parcel_pricing.price_card_version (tenant_id, registered_at DESC, plan_id DESC, plan_version DESC);

-- 参考序列登记册：缺省序登记时刻倒序，标识（序列、版本）。
CREATE INDEX reference_series_version_by_tenant_registered
    ON parcel_pricing.reference_series_version (tenant_id, registered_at DESC, series_id DESC, series_version DESC);

-- 参考序列覆盖：一行是（序列、种类），缺省序序列标识升序；本页的键按这条索引的次序去重取出，再回表只取本页各序列的明细。
CREATE INDEX reference_series_version_by_tenant_series_kind
    ON parcel_pricing.reference_series_version (tenant_id, series_id, kind);

-- 价卡草稿查阅：缺省序录入时刻倒序，标识（方案、版本）。
CREATE INDEX price_card_draft_by_tenant_submitted
    ON parcel_pricing.price_card_draft (tenant_id, submitted_at DESC, plan_id DESC, plan_version DESC);
