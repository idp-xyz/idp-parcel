-- 库区登记册（UC-WF-001 的库区修订；仓配履约 CONTEXT「库区」与「主数据登记」）：租户在仓下登记的库区，身份是
-- （租户，仓编码，库区编码）——库区编码在仓内才唯一，同一个编码在两个仓下是两个库区。每笔修订是一份完整内容——名称、
-- 库区用途、是否停用——另记登记责任方与登记时刻。
--
-- 键=租户+仓编码+库区编码+修订号，修订从 1 起连续递增；改名称、停用与再启用都是新修订行，绝不 UPDATE（ADR-0031 登记面纪律，
-- 同 0001 的仓修订）。「连续」与「用途不改」由应用层的登记用例判，库上只用主键守「同一修订号至多一笔」——并发提交同一修订号
-- 的两笔，靠它让至多一笔落册。
--
-- 本表不对 0001 的仓修订表建外键：仓修订表的主键含修订号，库区指名的是仓而不是仓的某一笔修订，没有一列可以引用；而「仓须
-- 已登记」由应用层的登记用例判（停用的仓也算已登记，登记册只追加，登记过的仓不会再消失），外键换不来应用层没有的保证。
--
-- 本迁移只建结构、不种任何行：库区是租户取值，产品不带默认库区（CONTEXT「主数据登记」，ADR-0375 决定五）。仓配开关关着的
-- 部署也施加本迁移，表是空的（ADR-0375 决定四）。
--
-- 仓编码、库区编码与库区名称逐字保存：库上只锁「不是空串或全空格」（btrim 的默认只去空格；制表符、换行等别的空白字符归领域
-- 构造器拦），主键按原样比较，不折大小写、不去首尾与内部空白，`Z-1`、`z-1` 与 ` Z-1` 是三个库区。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：本表的列都是 NOT NULL，没有可空列。

CREATE TABLE warehouse_fulfillment.zone_revision (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    zone_code      text        NOT NULL,
    revision       integer     NOT NULL,

    zone_name      text        NOT NULL,
    zone_usage     text        NOT NULL,
    deactivated    boolean     NOT NULL,

    registrant     text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT zone_revision_pkey
        PRIMARY KEY (tenant_id, warehouse_code, zone_code, revision),

    CONSTRAINT zone_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(zone_code) <> ''
            AND btrim(zone_name) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT zone_revision_revision_positive
        CHECK (revision >= 1),

    -- 镜像 domain.ZoneUsage 的封闭六种。新增一种用途先改领域封闭集，再改这一条。
    CONSTRAINT zone_revision_usage_closed
        CHECK (zone_usage IN (
            'RECEIVING_STAGING', 'STORAGE', 'PICKING', 'DEFECTIVE', 'RETURN_STAGING', 'OUTBOUND_STAGING'
        ))
);

-- 列表按（租户，仓编码，库区编码）取各库区的最新修订、取某个库区的整条修订链都经主键前缀定位，不另建索引。
