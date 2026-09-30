-- 可达性判断记录补当次解析依据里除判断键以外的两件（ADR-0075 决定三）：
-- 所携地理解析投影的版本化内容摘要，以及这次用过的服务区域版本。
-- 地址本体不进这张表。已有行没有这两件，摘要留空、版本清单留空数组。

ALTER TABLE network_routing.reachability_judgment
    ADD COLUMN geo_projection_digest text NOT NULL DEFAULT '',
    ADD COLUMN service_area_versions jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE network_routing.reachability_judgment
    ADD CONSTRAINT reachability_judgment_service_area_versions_shaped
        CHECK (jsonb_typeof(service_area_versions) = 'array');
