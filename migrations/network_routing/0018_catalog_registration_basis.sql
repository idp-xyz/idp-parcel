-- 版本化网络目录稳定定义各族的登记依据（票 routing-first-cut/11，ADR-0147 决定三、四）。
--
-- 每张版本表加一格可空的串：这一版凭什么登进来。采用随产品发布的参考配置时由采用路径写成
-- `REFCFG-1:<标识>@<版本>`；采用后改过的修订带租户自己的依据。空是这一版登记时没给依据——本迁移之前
-- 的存量行与不带依据的直接登记都是这一格，不回填、不补默认。依据格只是一列串而不是标识与版本两列，
-- 理由见 ADR-0147 候选丁。
--
-- 临时可用性调整不加这一格：它已有「来源」，是一条陈述，不经采用。线路段成本依据随线路版本的内容走，
-- 登记依据记在线路版本行上。

ALTER TABLE network_routing.logistics_node_version ADD COLUMN basis_ref text;
ALTER TABLE network_routing.network_connection_version ADD COLUMN basis_ref text;
ALTER TABLE network_routing.line_version ADD COLUMN basis_ref text;
ALTER TABLE network_routing.service_area_version ADD COLUMN basis_ref text;
ALTER TABLE network_routing.service_calendar_version ADD COLUMN basis_ref text;
ALTER TABLE network_routing.route_strategy_version ADD COLUMN basis_ref text;

ALTER TABLE network_routing.logistics_node_version
    ADD CONSTRAINT logistics_node_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
ALTER TABLE network_routing.network_connection_version
    ADD CONSTRAINT network_connection_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
ALTER TABLE network_routing.line_version
    ADD CONSTRAINT line_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
ALTER TABLE network_routing.service_area_version
    ADD CONSTRAINT service_area_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
ALTER TABLE network_routing.service_calendar_version
    ADD CONSTRAINT service_calendar_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
ALTER TABLE network_routing.route_strategy_version
    ADD CONSTRAINT route_strategy_version_basis_not_blank
        CHECK (basis_ref IS NULL OR btrim(basis_ref) <> '');
