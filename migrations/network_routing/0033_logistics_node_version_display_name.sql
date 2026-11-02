-- 物流节点的一版可以带一个可选的显示名称（ADR-0353）。名称只供人辨认：不是身份、不唯一，不进选版读口与任何路由判断的
-- 依据；改名是登一个新版本，旧版本照留它当时的名称。没登即 NULL，读口不带这一格，读者照旧以节点身份称呼它。
--
-- 存量安全：只加一个可空列，不回填、不改写任何既有行——迁移之前登下的版本读成没登名称，主键、区间、依据与目录修订
-- 一格不动。CHECK 对 NULL 恒成立，直接校验，不用 NOT VALID。
--
-- CHECK 是第二道网，只守 ASCII 空白与长度，宽严随库 ctype；完整校验以领域构造门（domain.NewNodeDisplayName）为准。
-- `\s` 与 `[[:cntrl:]]` 在 C.UTF-8 之类的 ctype 下不认 U+0085、U+00A0、U+2007、U+202F 等非 ASCII 空白，以不换行空格
-- 打头的名称过得了这道 CHECK；格式字符与行段分隔符也只由领域门拒。上限 100 个字符是产品策略（ADR-0353 决定三）。

ALTER TABLE network_routing.logistics_node_version
    ADD COLUMN display_name text;

ALTER TABLE network_routing.logistics_node_version
    ADD CONSTRAINT logistics_node_version_display_name_shape
        CHECK (display_name IS NULL OR (
            char_length(display_name) <= 100
            AND display_name ~ '\S'
            AND display_name !~ '^\s'
            AND display_name !~ '\s$'
            AND display_name !~ '[[:cntrl:]]'
        ));
