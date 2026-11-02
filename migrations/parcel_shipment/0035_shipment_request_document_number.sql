-- 委托的单据号（ADR-0355 决定三、七、八、十一；PS CONTEXT「委托」生命周期草稿 → 已提交那一句；票 document-numbering/04）。
--
-- 单据号是委托上另加的一项，不是委托标识：主键、委托标识唯一键与幂等一概不动。两列都是列而不进快照文档——
-- 号在建单那一笔事务里开立、此后不变，Save 改写快照不碰它们；放进文档就是第二个来源，一次只改文档的写入会让两处各说各话。
--
-- 一、document_number：开立时取到的单据号；没有单据号时为空。
-- 二、document_number_absence：开立时没取到号的原因，只收「单据编号时区未配置」一格。两列皆空即本迁移之前建的委托
--     （「上线前开立」）：旧行不回填、事后不补号（ADR-0355 决定三），所以那一格不必写成值，空着就是它。
--     有号与记了无号原因互斥：一份委托要么有号，要么说清为什么没有。
-- 三、（租户，单据号）唯一：同一序列分出的号互不相同，撞号只可能撞上换格式之前已用的号，由取号编排在开立事务里跳过
--     （决定八）；这条约束是最后一道保险，不是判撞的地方。跨租户不设唯一：两个租户各有一个同号委托是隔离的正常形态。
--     空值不参与唯一判定，旧行与无号委托都不占号。
--
-- 只加可空列、不回填，在有数据的库上直接施加。

ALTER TABLE parcel_shipment.shipment_request
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE parcel_shipment.shipment_request
    ADD CONSTRAINT shipment_request_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT shipment_request_document_number_absence_closed
        CHECK (document_number_absence IS NULL OR document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
    ADD CONSTRAINT shipment_request_document_number_or_absence
        CHECK (document_number IS NULL OR document_number_absence IS NULL),
    ADD CONSTRAINT shipment_request_document_number_unique
        UNIQUE (tenant_id, document_number);
