-- 处理尝试另记结算的未形成原因、这一版申报的服务产品与它属于哪一版（ADR-0380 决定三、四；票 declared-goods-lines/23）。
--
-- 价卡一侧答不出的估价改等运营登记之后，这一类停顿的处理尝试随等待态一起落库；可它上面只有小包托运自己的待判原因
-- （FINANCIAL_CONTROL_NOT_CONFIGURED），说不出是没有在用价卡、价卡适用冲突还是试算没有合计，也说不出是哪个服务产品没有卡。
-- 委托详情的「上次处理」也改从本表读当前这一版任务的最近一条，而本表一直没有版本：0018 当时只给两类判断加了版本列，
-- 理由是尝试不参与决定、续办引用的派生已含版本——可那个引用是摘要，读不回是哪一版。
--
-- 一、settlement_reason：结算答未形成时交回的原因码原样（如 ESTIMATE_PRICE_CARD_NOT_CONFIGURED），不冠 SA-——这一列本身就是
--     「来自结算」。结算没答未形成的停顿为空。取值出自结算的封闭集合，在场时不许空白。
-- 二、service_product：这一版申报的服务产品，取小包托运自己的提交版本，不经结算与计价转交；没申报为空。不加非空白约束：
--     申报值是客户的原话，本上下文原样保全、不去空白（领域 DeclaredServiceProduct），约束一旦拦下它，处理尝试的写入失败会
--     让整笔事务作废——recordAttempt 不上抛这一失败，消费门随后的提交落 publish_failed，码面指错方向（0011 头注量过同一幕）。
-- 三、submission_version：这一轮判的是哪一版。在场时不许空白。
--
-- 结算原因与服务产品只在落库的尝试上有意义：内部重试那一路整笔回滚，写了也不留。三列旧行一律不回填——本迁移之前落下的
-- 尝试当时没记这几格，事后推出来的值说的是今天的提交版本，不是那一轮停下时的；读口对版本为空的旧行按任务建立时刻圈定。
--
-- 只加可空列、不回填，在有数据的库上直接施加。

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    ADD COLUMN settlement_reason text,
    ADD COLUMN service_product text,
    ADD COLUMN submission_version text;

ALTER TABLE parcel_shipment.acceptance_processing_attempt
    ADD CONSTRAINT acceptance_processing_attempt_settlement_reason_not_blank
        CHECK (settlement_reason IS NULL OR btrim(settlement_reason) <> ''),
    ADD CONSTRAINT acceptance_processing_attempt_submission_version_not_blank
        CHECK (submission_version IS NULL OR btrim(submission_version) <> '');
