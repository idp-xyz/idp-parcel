-- 外部标识关联的渠道来源与适用状态变化（ADR-0184 决定三第二项、决定七；票 ps-external-mark-relations/03）。
--
-- 一、类型封闭集加渠道包裹件号：渠道在面单交易包裹结果里签发的包裹级标识，标识包裹。
--
-- 二、适用变化另立一张表，挂在原关联上：原关联一行不改，「已作废」「已被替代」是这里的一行。主键就是关联键，
-- 所以一条关联至多一个终态——终态不回到适用，第二次变化（先作废、后又被替代）写不进来，ON CONFLICT DO NOTHING
-- 让先到的那一格立住。已失效没有执行器（有效期规则无登记处），封闭集里不放。
--
-- 替代要指名后继：后继与原关联同租户、同类型、同包裹，变的只是分配方与号码，两列即可回指，不另存整条键。

ALTER TABLE parcel_shipment.external_identifier_relation
    DROP CONSTRAINT external_identifier_relation_kind_closed,
    ADD CONSTRAINT external_identifier_relation_kind_closed
        CHECK (identifier_kind IN ('CUSTOMER_PARCEL_NUMBER', 'CHANNEL_PARCEL_NUMBER')),
    DROP CONSTRAINT external_identifier_relation_kind_names_its_object,
    ADD CONSTRAINT external_identifier_relation_kind_names_its_object
        CHECK (identifier_kind NOT IN ('CUSTOMER_PARCEL_NUMBER', 'CHANNEL_PARCEL_NUMBER') OR object_kind = 'PARCEL');

-- 适用变化按（租户，类型，形成依据，包裹）选中原关联：渠道包裹件号由形成它的交易与包裹唯一确定。
CREATE INDEX external_identifier_relation_by_basis
    ON parcel_shipment.external_identifier_relation (tenant_id, identifier_kind, basis, object_ref);

CREATE TABLE parcel_shipment.external_identifier_relation_change (
    tenant_id           text        NOT NULL,
    identifier_kind     text        NOT NULL,
    allocator           text        NOT NULL,
    identifier_number   text        NOT NULL,
    object_ref          text        NOT NULL,
    change_kind         text        NOT NULL,
    successor_allocator text,
    successor_number    text,
    -- 带来这次变化的那笔来源事实：作废是被作废的交易本身，替代是新交易。
    cause               text        NOT NULL,
    changed_at          timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_identifier_relation_change_pkey
        PRIMARY KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref),
    CONSTRAINT external_identifier_relation_change_of_a_relation
        FOREIGN KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref)
        REFERENCES parcel_shipment.external_identifier_relation
            (tenant_id, identifier_kind, allocator, identifier_number, object_ref),
    -- 封闭集与领域 ExternalIdentifierChangeKind 同笔改。
    CONSTRAINT external_identifier_relation_change_kind_closed
        CHECK (change_kind IN ('VOIDED', 'REPLACED')),
    -- 只有已被替代带后继，且两列同在同缺。
    CONSTRAINT external_identifier_relation_change_successor_only_when_replaced
        CHECK (
            (change_kind = 'REPLACED') = (successor_allocator IS NOT NULL)
            AND (successor_allocator IS NULL) = (successor_number IS NULL)
        ),
    CONSTRAINT external_identifier_relation_change_cause_not_blank
        CHECK (btrim(cause) <> '')
);
