-- 仓登记册（UC-WF-001 的仓修订；仓配履约 CONTEXT「主数据登记」与「身份与边界」）：租户登记的仓，身份是
-- （租户，仓编码），每笔修订是一份完整内容——名称、自营仓或代理仓、地址、业务时区（可缺）、同址物流节点（可缺）、
-- 是否停用——另记登记责任方与登记时刻。
--
-- 键=租户+仓编码+修订号，修订从 1 起连续递增；改资料、停用与再启用都是新修订行，绝不 UPDATE（ADR-0031 登记面纪律，
-- 同 party_commercial 的 0033 目录）。「连续」与「种类不改」由应用层的登记用例判，库上只用主键守「同一修订号至多
-- 一笔」——并发提交同一修订号的两笔，靠它让至多一笔落册。
--
-- 本迁移只建结构、不种任何行：仓是租户取值，产品不带默认仓（CONTEXT「主数据登记」，ADR-0375 决定五）。仓配开关关着的
-- 部署也施加本迁移，表是空的（ADR-0375 决定四）。
--
-- 仓编码、地址正文与同址物流节点逐字保存：库上只锁「不是空串或全空格」（btrim 的默认只去空格；制表符、换行等别的空白
-- 字符归领域构造器拦），主键按原样比较，不折大小写、不去首尾与内部空白，`WH-1`、`wh-1` 与 ` WH-1` 是三个仓。
-- 业务时区缺就是没登记，存 NULL，不取任何默认值。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。

CREATE TABLE warehouse_fulfillment.warehouse_revision (
    tenant_id          text        NOT NULL,
    warehouse_code     text        NOT NULL,
    revision           integer     NOT NULL,

    warehouse_name     text        NOT NULL,
    kind               text        NOT NULL,
    country_code       text        NOT NULL,
    address_text       text        NOT NULL,
    business_time_zone text,
    colocated_node     text,
    deactivated        boolean     NOT NULL,

    registrant         text        NOT NULL,
    registered_at      timestamptz NOT NULL,

    CONSTRAINT warehouse_revision_pkey
        PRIMARY KEY (tenant_id, warehouse_code, revision),

    CONSTRAINT warehouse_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(warehouse_name) <> ''
            AND btrim(country_code) <> ''
            AND btrim(address_text) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT warehouse_revision_revision_positive
        CHECK (revision >= 1),

    -- 镜像 domain.WarehouseKind 的封闭集。新增一种仓先改领域封闭集，再改这一条。
    CONSTRAINT warehouse_revision_kind_closed
        CHECK (kind IN ('SELF_OPERATED', 'AGENT')),

    -- 可缺的两格：要么没有（NULL），要么是非空的取值；空串不兼作「没登记」「没指名」。
    CONSTRAINT warehouse_revision_optional_cells_not_blank
        CHECK (
            (business_time_zone IS NULL OR btrim(business_time_zone) <> '')
            AND (colocated_node IS NULL OR btrim(colocated_node) <> '')
        )
);

-- 列表按（租户，仓编码）取各仓的最新修订、取某个仓的整条修订链都经主键前缀定位，不另建索引。
