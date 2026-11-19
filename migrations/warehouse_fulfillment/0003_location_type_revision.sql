-- 库位类型登记册（UC-WF-001 的库位类型修订；仓配履约 CONTEXT「库位」与「主数据登记」）：租户登记的库位类型（货架、地堆、托盘位之类），
-- 按租户登记、各仓共用，身份是（租户，库位类型编码），所以键里没有仓编码。每笔修订是一份完整内容——名称、是否停用——另记登记责任方
-- 与登记时刻。
--
-- 键=租户+库位类型编码+修订号，修订从 1 起连续递增；改名称、停用与再启用都是新修订行，绝不 UPDATE（ADR-0031 登记面纪律，同 0001 的仓
-- 修订）。「连续」与「本租户已登记过仓」由应用层的登记用例判，库上只用主键守「同一修订号至多一笔」——并发提交同一修订号的两笔，靠它
-- 让至多一笔落册。库位类型没有登记后不改的格，也没有指名别的对象的格。
--
-- 本表不对 0001 的仓修订表建外键：库位类型不指名仓，「本租户一个仓都没登记就答未配置」是应用层的答法，库上没有一列可以引用。
--
-- 本迁移只建结构、不种任何行：库位类型是租户取值，产品不内置任何库位类型（CONTEXT「主数据登记」，ADR-0375 决定五）。仓配开关关着的
-- 部署也施加本迁移，表是空的（ADR-0375 决定四）。
--
-- 库位类型编码与名称逐字保存：库上只锁「不是空串或全空格」（btrim 的默认只去空格；制表符、换行等别的空白字符归领域构造器拦），
-- 主键按原样比较，不折大小写、不去首尾与内部空白，`RACK`、`rack` 与 ` RACK` 是三个库位类型。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：本表的列都是 NOT NULL，没有可空列。

CREATE TABLE warehouse_fulfillment.location_type_revision (
    tenant_id          text        NOT NULL,
    location_type_code text        NOT NULL,
    revision           integer     NOT NULL,

    location_type_name text        NOT NULL,
    deactivated        boolean     NOT NULL,

    registrant         text        NOT NULL,
    registered_at      timestamptz NOT NULL,

    CONSTRAINT location_type_revision_pkey
        PRIMARY KEY (tenant_id, location_type_code, revision),

    CONSTRAINT location_type_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(location_type_code) <> ''
            AND btrim(location_type_name) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT location_type_revision_revision_positive
        CHECK (revision >= 1)
);

-- 列表按（租户，库位类型编码）取各库位类型的最新修订、取某个库位类型的整条修订链都经主键前缀定位，不另建索引。
