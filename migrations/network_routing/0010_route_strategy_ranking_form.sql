-- 路由策略版本声明的排序形态（ADR-0146 决定二、七）。0008 建表时写「规则正文全属 PAR-NET-14，
-- 不设内容列」；ADR-0146 把排序形态判为产品策略——形态集合与判断逻辑归产品，租户只选形态——
-- 这一格于是成了版本自带的声明。可空：NULL 即这一版没有声明形态，排序如实答未配置，不替租户
-- 选一种；本迁移之前登记的版本行照旧是 NULL。
--
-- CHECK 与领域 RankingForm 的形态集合逐字同格，是第二道网：形态族加一种时两处一起放宽。
ALTER TABLE network_routing.route_strategy_version
    ADD COLUMN ranking_form text;

ALTER TABLE network_routing.route_strategy_version
    ADD CONSTRAINT route_strategy_version_ranking_form_known
    CHECK (ranking_form IS NULL OR ranking_form IN ('COST_SINGLE_DIMENSION'));
