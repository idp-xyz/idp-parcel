-- 价格政策口径册退掉体积一格（ADR-0260 决定二，票 docs-consistency/code/25）：体积系数与金额取整是价卡内容、
-- 不分方向，商业价格政策任何方向都不在口径上声明。
--
-- 0022 把 volumetric_factor_ref 与 direction 充要耦合（SELL 必有、BUY/INTERNAL 必无）。那条耦合建立在「销售方向
-- 没有承运商卡、只能由商业政策声明系数」上；ADR-0260 裁定 SELL 卡与 BUY 卡同法声明系数，评价只读卡上那一处，
-- 于是政策上这一格无论哪个方向都成了一处可以合法声明却静默不起作用的副本——租户按合同调了政策上的系数而不改卡，
-- 计费重量就与预期不符而没有任何地方报错。撤掉耦合，换成任何方向都只收 NULL。
--
-- 不改 0022：已施加的迁移正文按校验和守着（ADR-0068 Consequences）；耦合的理由那段头注留在 0022 上作历史，现行
-- 规则由本迁移与 domain.PricePolicyCaliber 的注释承载。0022 头注提到的领域方向核 ConsistentWithDirection 已删：口径的
-- 方向由发布编排从政策正文抄来，方向一致只由 0022 的复合外键 price_policy_caliber_policy_fkey 在库上守。
--
-- 新 CHECK 以 NOT VALID 加上：按 0022 登下的销售行带着系数，原样留在册上作历史——读面把那一格标
-- 「不参与计价」（ports.PricePolicyRow.HistoricalVolumetricFactor），不删行、不抹列。NOT VALID 只管此后的 INSERT 与
-- UPDATE，不校存量，正是「历史行留册、新值拒收」这一句在库上的写法。列本身不删：删列会连历史一起抹掉，而那几
-- 行的系数是当年真实登记过的声明。
--
-- 0022 的 price_policy_caliber_not_blank 对这一列的非空白判仍成立（NULL 过、空串拒），不动。

ALTER TABLE party_commercial.price_policy_caliber
    DROP CONSTRAINT price_policy_caliber_volumetric_factor_coupled;

ALTER TABLE party_commercial.price_policy_caliber
    ADD CONSTRAINT price_policy_caliber_volumetric_factor_retired
        CHECK (volumetric_factor_ref IS NULL) NOT VALID;
