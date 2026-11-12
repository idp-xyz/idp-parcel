-- 处理尝试另记申报不全那一轮计价点名的缺项码（ADR-0381 决定四；票 declared-goods-lines/26）。
--
-- 申报不全自本票起按缺项分流：客户补得上的停`等待受控补充`，登记才解得开的照旧停`等待运营登记`。两路的处理尝试
-- 都只记得本上下文的待判原因与结算的原因码（ESTIMATE_DECLARATION_INCOMPLETE），说不出缺的是哪几格——运营排查
-- 「这一轮为什么停」与管理台的「上次处理」要的正是计价当时点名的那几个码（MEASUREMENT、DIMENSIONS、POSTAL_ROUTE、
-- ORIGIN_POSTAL、ZONE、ADDRESS_NATURE……）。等待受控补充那一路的任务上另有译后的缺口（快照文档 supplementGaps），
-- 但那是任务当前态、换代即清；尝试行是逐轮留痕，两者不互相顶替，等运营登记那一路也只有这一格可读。
--
-- estimate_gap_codes：计价缺项码原样、先见先记去重，一个数组列装整轮——缺项是这一轮的一份点名，不是逐码各一行
-- 的事实（判据同 0006 的 declared_parcel_ids）。只在申报不全的那两路等待上有；其余停法写 NULL，不写空数组——
-- 空数组会被读成「点名了零项」。在场时不许空数组，空白元素由领域构造门拒（写进来会让整笔事务作废，0038 头注
-- 量过同一幕），列上只守住「在场即非空」。
--
-- 旧行一律不回填：本迁移之前落下的尝试当时没记这一格，事后从今天的申报推出来的码说的不是那一轮停下时的。
-- 读口（lastAttempts）对缺席读回缺席。
--
-- 只加可空列、不回填，在有数据的库上直接施加。

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    ADD COLUMN estimate_gap_codes text[];

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    ADD CONSTRAINT acceptance_processing_attempt_estimate_gap_codes_not_empty
        CHECK (estimate_gap_codes IS NULL OR cardinality(estimate_gap_codes) >= 1);
