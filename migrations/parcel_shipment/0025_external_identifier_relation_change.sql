-- 外部标识关联的渠道来源与适用状态变化（ADR-0184 决定三第二项、决定七；票 ps-external-mark-relations/03）。
--
-- 一、类型封闭集加渠道包裹件号：渠道在面单交易包裹结果里签发的包裹级标识，标识包裹。
--
-- 二、适用变化另立一张表，挂在原关联上：原关联一行不改，「已作废」「已被替代」是这里的一行。主键就是关联键，
-- 所以一条关联至多一个终态——终态不回到适用，第二次变化（先作废、后又被替代）写不进来，ON CONFLICT DO NOTHING
-- 让先到的那一格立住。已失效没有执行器（有效期规则无登记处），封闭集里不放。
--
-- 三、形成依据并进关联键（ADR-0185 修订 ADR-0184 决定一）：同一分配方对同一件包裹再签同一个号——换单仍沿用原号，
-- 或号作废后由下一笔交易再签——是一条新的关联，不能被旧键吞掉。旧键下，换单沿用原号时「已被替代」的后继会指回
-- 被替代者自己，那件包裹从此认不出来；作废后再签的号也形不成新关联，违反「同一号码后来重新标识同一对象时形成新的
-- 关联，不复活旧的」。0024 落下的行键值不变，只是主键多一列。
--
-- 替代要指名后继：后继与原关联同租户、同类型、同包裹，变的是分配方、号码与形成依据，三列即可回指，不另存整条键。

ALTER TABLE parcel_shipment.external_identifier_relation
    DROP CONSTRAINT external_identifier_relation_pkey,
    ADD CONSTRAINT external_identifier_relation_pkey
        PRIMARY KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis),
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
    basis               text        NOT NULL,
    change_kind         text        NOT NULL,
    successor_allocator text,
    successor_number    text,
    successor_basis     text,
    -- 带来这次变化的那笔来源事实：作废是被作废的交易本身，替代是新交易。
    cause               text        NOT NULL,
    changed_at          timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_identifier_relation_change_pkey
        PRIMARY KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis),
    CONSTRAINT external_identifier_relation_change_of_a_relation
        FOREIGN KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis)
        REFERENCES parcel_shipment.external_identifier_relation
            (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis),
    -- 封闭集与领域 ExternalIdentifierChangeKind 同笔改。
    CONSTRAINT external_identifier_relation_change_kind_closed
        CHECK (change_kind IN ('VOIDED', 'REPLACED')),
    -- 只有已被替代带后继，且三列同在同缺；后继必是另一条关联——指回自己的替代没有意义。
    CONSTRAINT external_identifier_relation_change_successor_only_when_replaced
        CHECK (
            (change_kind = 'REPLACED') = (successor_allocator IS NOT NULL)
            AND (successor_allocator IS NULL) = (successor_number IS NULL)
            AND (successor_allocator IS NULL) = (successor_basis IS NULL)
        ),
    CONSTRAINT external_identifier_relation_change_successor_is_another_relation
        CHECK (successor_basis IS NULL OR successor_basis <> basis),
    -- 照 0024 refs_not_blank 的先例：在场的引用不许是空白。
    CONSTRAINT external_identifier_relation_change_refs_not_blank
        CHECK (
            btrim(basis) <> ''
            AND btrim(cause) <> ''
            AND (successor_allocator IS NULL OR btrim(successor_allocator) <> '')
            AND (successor_number IS NULL OR btrim(successor_number) <> '')
            AND (successor_basis IS NULL OR btrim(successor_basis) <> '')
        )
);
