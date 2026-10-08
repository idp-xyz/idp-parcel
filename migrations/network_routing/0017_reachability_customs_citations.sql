-- 可达性判断记录补关务事实的逐候选出处（ADR-0148 决定一「端口取回的事实带出处、与
-- 判断一并留痕」，票 routing-first-cut/12）：判断标识与所用口岸/申报路径目录版本引用。
-- 已有行没有出处，留空数组——来源未接时出处缺席是如实形态，不是判断缺件。

ALTER TABLE network_routing.reachability_judgment
    ADD COLUMN customs_citations jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE network_routing.reachability_judgment
    ADD CONSTRAINT reachability_judgment_customs_citations_shaped
        CHECK (jsonb_typeof(customs_citations) = 'array');