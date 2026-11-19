-- 包材登记册（UC-WF-001 的包材修订；仓配履约 CONTEXT「包材」与「主数据登记」）：租户登记的包装材料（纸箱、袋、填充之类），按租户登记、
-- 各仓共用，身份是（租户，包材编码），所以键里没有仓编码。每笔修订是一份完整内容——名称、外廓尺寸（长、宽、高与一个长度单位）、重量
-- （一个数值与一个重量单位）、是否停用——另记登记责任方与登记时刻。
--
-- 键=租户+包材编码+修订号，修订从 1 起连续递增；改资料、停用与再启用都是新修订行，绝不 UPDATE（ADR-0031 登记面纪律，同 0001 的仓修订）。
-- 「连续」与「本租户已登记过仓」由应用层的登记用例判，库上只用主键守「同一修订号至多一笔」——并发提交同一修订号的两笔，靠它让至多一笔
-- 落册。包材没有登记后不改的格，也没有指名别的对象的格。
--
-- 本表不对 0001 的仓修订表建外键：包材不指名仓，「本租户一个仓都没登记就答未配置」是应用层的答法，库上没有一列可以引用。
--
-- 本迁移只建结构、不种任何行：包材是租户取值，产品不内置任何包材（CONTEXT「主数据登记」，ADR-0375 决定五）。仓配开关关着的部署也施加
-- 本迁移，表是空的（ADR-0375 决定四）。
--
-- 包材编码、名称与单位码逐字保存：库上只锁「不是空串或全空格」（btrim 的默认只去空格；制表符、换行等别的空白字符归领域构造器拦），
-- 主键按原样比较，不折大小写、不去首尾与内部空白，`BOX`、`box` 与 ` BOX` 是三个包材。登记时不折算单位：30 CM 与 0.3 M 是两行不同的内容。
--
-- 四个数值按精确十进制存（numeric，不用浮点）：整数部分至多 9 位、小数部分至多 6 位，numeric(15, 6) 恰好装得下，与领域里的写法门
-- 同一个上限；落库的值是登记时的数值，不带写法，12.5 与 12.50 是同一个值。数值列的 CHECK 守「为正」：为零的内容在应用层的登记门就被拒，
-- 库上再兜一道，直接写库也写不进零与负数。
--
-- 单位码在不在产品内置的计量单位目录里、是长度类还是重量类，不在库上镜像：目录随代码发布、会增补，镜像进库就让每次增补多一次迁移，而这是
-- 只拦新修订的登记门，读回的在册行不受目录更新牵连（同 0001 的国家码）。库上只锁单位码不是空白。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：本表的列都是 NOT NULL，没有可空列。

CREATE TABLE warehouse_fulfillment.packaging_material_revision (
    tenant_id               text          NOT NULL,
    packaging_material_code text          NOT NULL,
    revision                integer       NOT NULL,

    packaging_material_name text          NOT NULL,
    length_value            numeric(15,6) NOT NULL,
    width_value             numeric(15,6) NOT NULL,
    height_value            numeric(15,6) NOT NULL,
    dimensions_unit         text          NOT NULL,
    weight_value            numeric(15,6) NOT NULL,
    weight_unit             text          NOT NULL,
    deactivated             boolean       NOT NULL,

    registrant              text          NOT NULL,
    registered_at           timestamptz   NOT NULL,

    CONSTRAINT packaging_material_revision_pkey
        PRIMARY KEY (tenant_id, packaging_material_code, revision),

    CONSTRAINT packaging_material_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(packaging_material_code) <> ''
            AND btrim(packaging_material_name) <> ''
            AND btrim(dimensions_unit) <> ''
            AND btrim(weight_unit) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT packaging_material_revision_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT packaging_material_revision_values_positive
        CHECK (
            length_value > 0
            AND width_value > 0
            AND height_value > 0
            AND weight_value > 0
        )
);

-- 列表按（租户，包材编码）取各包材的最新修订、取某个包材的整条修订链都经主键前缀定位，不另建索引。
