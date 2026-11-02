-- 口岸目录的一版可以带一个可选的显示名称（ADR-0353）。名称只供人辨认：不是标识、不唯一，不进关务适用性判断的作答、
-- 出处与判断标识；改名是登一个起点更晚的新版本，旧版本照留它当时的名称。没登即 NULL，读口不带这一格。
--
-- 名称是这一版的登记内容：同一起点重登时由编排读回、与请求逐字段比对（NULL 与没给名称相同），名称不同即内容冲突，
-- 不覆盖、不补登——写口照旧不 UPSERT，本迁移之后也没有任何改写名称的路径。
--
-- 存量安全：只加一个可空列，不回填、不改写任何既有行——迁移之前登下的版本读成没登名称，键、区间与排他约束一格不动。
-- CHECK 对 NULL 恒成立，直接校验，不用 NOT VALID。CHECK 是第二道网，第一道是领域构造门
-- （domain.NewCustomsPortDisplayName）；上限 100 个字符是产品策略（ADR-0353 决定三）。

ALTER TABLE customs_compliance.candidate_port
    ADD COLUMN display_name text;

ALTER TABLE customs_compliance.candidate_port
    ADD CONSTRAINT candidate_port_display_name_shape
        CHECK (display_name IS NULL OR (
            char_length(display_name) <= 100
            AND display_name ~ '\S'
            AND display_name !~ '^\s'
            AND display_name !~ '\s$'
            AND display_name !~ '[[:cntrl:]]'
        ));
