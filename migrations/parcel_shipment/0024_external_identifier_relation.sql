-- 外部标识关联册（ADR-0184 决定一）。
--
-- 一行一条关联：哪个租户里，哪一类号码、由谁分配、号码是什么、标识了哪个对象，在哪个编号空间里算数，凭什么
-- 形成、何时形成。**只插不改**：没有乐观版本列，也没有 state 列——适用状态的变化（作废、替代、失效）是挂在
-- 原关联上的另一行，随渠道来源那一票的迁移进来；今天的来源只有客户包裹件号，它没有作废或替代的来源事实。
--
-- 主键即关联身份。同一个号码可以有多条：分配方不同、对象不同，或前后替代——所以号码不是唯一键，
-- 「一号一包裹」的映射表正是 PS CONTEXT「不能全部解释为包裹的当前运单号」那句禁的形状。
--
-- 号码逐字存、逐字比：不 lower、不 btrim、不去前导零。条码规则属租户取值（UC-NO-002），库里不替它折算；
-- 只拦全空白，那不是任何参与方分配得出的号。

CREATE TABLE parcel_shipment.external_identifier_relation (
    tenant_id         text        NOT NULL,
    identifier_kind   text        NOT NULL,
    allocator         text        NOT NULL,
    identifier_number text        NOT NULL,
    object_kind       text        NOT NULL,
    object_ref        text        NOT NULL,
    -- 适用范围单列，不由分配方推出：今天与分配方同值，更窄的范围日后加列（ADR-0184 决定六）。
    scope             text        NOT NULL,
    basis             text        NOT NULL,
    formed_at         timestamptz NOT NULL,
    recorded_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT external_identifier_relation_pkey
        PRIMARY KEY (tenant_id, identifier_kind, allocator, identifier_number, object_ref),

    CONSTRAINT external_identifier_relation_refs_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(allocator) <> ''
            AND btrim(identifier_number) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(scope) <> ''
            AND btrim(basis) <> ''
        ),
    -- 类型封闭集与领域 ExternalIdentifierKind 同笔改。
    CONSTRAINT external_identifier_relation_kind_closed
        CHECK (identifier_kind IN ('CUSTOMER_PARCEL_NUMBER')),
    CONSTRAINT external_identifier_relation_object_kind_closed
        CHECK (object_kind IN ('PARCEL')),
    -- 类型决定对象种类（ADR-0184 决定二）：号码挂错对象在库里也立不起来。
    CONSTRAINT external_identifier_relation_kind_names_its_object
        CHECK (identifier_kind <> 'CUSTOMER_PARCEL_NUMBER' OR object_kind = 'PARCEL')
);

-- 身份核对按（租户，号码）查，跨类型、跨分配方：现场观察只带一个号码。
CREATE INDEX external_identifier_relation_by_number
    ON parcel_shipment.external_identifier_relation (tenant_id, identifier_number);
