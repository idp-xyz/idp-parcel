-- 库位登记册（UC-WF-001 的库位修订；仓配履约 CONTEXT「库位」与「主数据登记」）：租户在仓下登记的库位，身份是（租户，仓编码，库位编码）——
-- 库位编码在仓内才唯一，同一个编码在两个仓下是两个库位。每笔修订是一份完整内容——所属库区、库位类型（可缺）、优先级（可缺）、
-- 是否停用——另记登记责任方与登记时刻。
--
-- 键=租户+仓编码+库位编码+修订号，修订从 1 起连续递增；改库位类型、改优先级、停用与再启用都是新修订行，绝不 UPDATE（ADR-0031 登记面
-- 纪律，同 0001 的仓修订）。「连续」「所属库区不改」与各指名对象的核对由应用层的登记用例判，库上只用主键守「同一修订号至多一笔」——
-- 并发提交同一修订号的两笔，靠它让至多一笔落册。
--
-- 本表不对 0001 的仓修订表、0002 的库区修订表与 0003 的库位类型修订表建外键：它们的主键都含修订号，库位指名的是仓、库区与库位类型
-- 而不是它们的某一笔修订，没有一列可以引用；而「已登记」由应用层的登记用例判（停用的也算已登记，各册只追加，登记过的对象不会再
-- 消失），外键换不来应用层没有的保证。
--
-- 本迁移只建结构、不种任何行：库位是租户取值，产品不带默认库位（CONTEXT「主数据登记」，ADR-0375 决定五）。仓配开关关着的部署
-- 也施加本迁移，表是空的（ADR-0375 决定四）。
--
-- 仓编码、库位编码、所属库区编码与库位类型编码逐字保存：库上只锁「不是空串或全空格」（btrim 的默认只去空格；制表符、换行等别的空白
-- 字符归领域构造器拦），主键按原样比较，不折大小写、不去首尾与内部空白，`L-1`、`l-1` 与 ` L-1` 是三个库位。
--
-- 库位类型与优先级是可空列，NULL 就是「没有」：没有与有是不同的内容，优先级 0 与没有优先级是两份内容，库里不取任何默认值。
-- 全部 CHECK 过 SQL 三值逻辑那一眼：必填列都是 NOT NULL；可空的库位类型编码那一条先走 IS NULL 的一支——NULL 时那一支为真，整条
-- 约束为真、放行；不为 NULL 时才比较「不是空串或全空格」。优先级列的类型 integer 就是 32 位有符号整数，范围由类型本身守住，不另写 CHECK。

CREATE TABLE warehouse_fulfillment.location_revision (
    tenant_id          text        NOT NULL,
    warehouse_code     text        NOT NULL,
    location_code      text        NOT NULL,
    revision           integer     NOT NULL,

    zone_code          text        NOT NULL,
    location_type_code text,
    priority           integer,
    deactivated        boolean     NOT NULL,

    registrant         text        NOT NULL,
    registered_at      timestamptz NOT NULL,

    CONSTRAINT location_revision_pkey
        PRIMARY KEY (tenant_id, warehouse_code, location_code, revision),

    CONSTRAINT location_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(location_code) <> ''
            AND btrim(zone_code) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT location_revision_location_type_not_blank
        CHECK (location_type_code IS NULL OR btrim(location_type_code) <> ''),

    CONSTRAINT location_revision_revision_positive
        CHECK (revision >= 1)
);

-- 列表按（租户，仓编码，库位编码）取各库位的最新修订、取某个库位的整条修订链都经主键前缀定位，不另建索引。
