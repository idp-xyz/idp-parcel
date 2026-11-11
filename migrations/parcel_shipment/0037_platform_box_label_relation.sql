-- 外部标识关联的类型封闭集加平台箱标号（ADR-0374 决定四；票 declared-goods-lines/03）。
--
-- 销售平台给每一箱的箱标号（如亚马逊 FBA 箱标）：客户在委托里为声明包裹报上，委托接受时形成关联，标识包裹。分配方与适用
-- 范围是委托所报平台货件的销售平台，不是客户账户。与客户包裹件号同一套：委托`已提交`期间，被声明的箱标号在按号核对时同样
-- 算歧义信号：读口按包含查快照里的 platformBoxLabels，下面的部分索引与 0026 同形，只收`已提交`的行。

ALTER TABLE parcel_shipment.external_identifier_relation
    DROP CONSTRAINT external_identifier_relation_kind_closed,
    ADD CONSTRAINT external_identifier_relation_kind_closed
        CHECK (identifier_kind IN ('CUSTOMER_PARCEL_NUMBER', 'CHANNEL_PARCEL_NUMBER', 'PLATFORM_BOX_LABEL')),
    DROP CONSTRAINT external_identifier_relation_kind_names_its_object,
    ADD CONSTRAINT external_identifier_relation_kind_names_its_object
        CHECK (identifier_kind NOT IN ('CUSTOMER_PARCEL_NUMBER', 'CHANNEL_PARCEL_NUMBER', 'PLATFORM_BOX_LABEL') OR object_kind = 'PARCEL');

CREATE INDEX shipment_request_submitted_platform_box_labels
    ON parcel_shipment.shipment_request
    USING GIN ((snapshot -> 'platformBoxLabels') jsonb_path_ops)
    WHERE state = 1;
