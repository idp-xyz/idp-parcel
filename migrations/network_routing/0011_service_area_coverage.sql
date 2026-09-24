-- 服务区域版本加覆盖与节点角色（票 routing-first-cut/07；ADR-0148 决定二、五）。
--
-- CONTEXT Language「服务区域」：带版本和适用期的地理覆盖定义，「用于把客户地址解析为候选收寄节点、交付节点
-- 或尾程注入节点」——覆盖与节点角色随区域版本走，改哪一格都形成新版本，不覆盖原版本。
--
-- 覆盖文法首版两种形态：整个国家 / 地区（只填 coverage_country），或国家 / 地区加一组邮编前缀
-- （coverage_postal_prefixes 为非空数组）。形态与匹配规则归产品（domain.ServiceAreaCoverage），取值是租户的
-- 目录内容，本迁移不种任何默认行。
--
-- 四列全可空：本格落地之前的存量版本没有它们，读回照样成立；没登覆盖的区域不解析任何地址，候选在那一格如实
-- 不可用，而不是被猜成覆盖。库只钉单看一行就判得出的形状：国家码两位大写；前缀与两组节点都是非空数组；前缀与
-- 节点角色不脱离覆盖国家单独出现。数组内去重归登记用例（经领域构造门），节点在目录里是否存在要跨表判，归折叠。
-- 存量行四列全空，新 CHECK 对它们恒真；ADD CONSTRAINT 默认校验存量行，这一点由它自己证。
--
-- 与 0008 头注的关系：那份头注写于立表时——「在解析层存在之前，三个证据视图不读本目录」「视图修订与目录修订
-- 的合流属解析层设计」，折叠语义与内容列「全属 PAR-NET-14」。自本迁移所在的那一笔起按 ADR-0148 决定六部分
-- 停用：候选生成与折叠是产品策略（ADR-0146 决定七），可达性证据视图从本目录折出事实；`未配置`由目录修订锚与
-- 判断时点适用于该服务目的的路由策略版本答，视图修订就是 network_catalog_revision 的值；0007 的定义登记册不再
-- 被读。0008 已施加、按 checksum 固定不改，以本头注、适配器 NetworkCatalog 的类型注释与 ADR-0148 为准。

ALTER TABLE network_routing.service_area_version
    ADD COLUMN coverage_country         text,
    ADD COLUMN coverage_postal_prefixes jsonb,
    ADD COLUMN origin_node_codes        jsonb,
    ADD COLUMN destination_node_codes   jsonb;

ALTER TABLE network_routing.service_area_version
    ADD CONSTRAINT service_area_version_coverage_country_shape
        CHECK (coverage_country IS NULL OR coverage_country ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT service_area_version_postal_prefixes_shape
        CHECK (
            coverage_postal_prefixes IS NULL
            OR (coverage_country IS NOT NULL
                AND jsonb_typeof(coverage_postal_prefixes) = 'array'
                AND jsonb_array_length(coverage_postal_prefixes) > 0)
        ),
    ADD CONSTRAINT service_area_version_origin_nodes_shape
        CHECK (
            origin_node_codes IS NULL
            OR (coverage_country IS NOT NULL
                AND jsonb_typeof(origin_node_codes) = 'array'
                AND jsonb_array_length(origin_node_codes) > 0)
        ),
    ADD CONSTRAINT service_area_version_destination_nodes_shape
        CHECK (
            destination_node_codes IS NULL
            OR (coverage_country IS NOT NULL
                AND jsonb_typeof(destination_node_codes) = 'array'
                AND jsonb_array_length(destination_node_codes) > 0)
        );
