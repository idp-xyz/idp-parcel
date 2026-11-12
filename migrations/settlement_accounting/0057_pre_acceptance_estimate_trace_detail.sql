-- 票 declared-goods-lines/19（ADR-0379 决定一、二）：受理前估价留痕补记选卡用的服务产品、逐件计价重量与费用行，结算按委托交出读口。
--
-- 服务产品是一列（可空：没申报即不记）。计价重量与费用行补进 trials 的逐件 jsonb，键是 pricingWeight 与 chargeLines，缺即没记——
-- 一件的明细与它的评价标识、合计同在一个元素里，复算与读口都按件取，拆成第二张表只会多一处要对齐的件序。
--
-- detail_recorded 说这一行是不是补记之后写的。旧行不回填（决定一「补记之前的留痕答未记」），列加上时一律是 false；之后的写口必须
-- 显式写它，所以默认值随即去掉——留着默认，漏写的写口会把新行悄悄记成旧行。它不能从别的列推：没申报产品的新行同样没有产品，费用行
-- 也可能因一行译不出而整份不记，「缺」说不出是哪一种缺。
--
-- 只增不改照旧：同键再写，新行按全部内容比，旧行只比它当时有的那几格（重驱跨过本迁移的那一次控制请求要答重放，不答冲突）。

ALTER TABLE settlement_accounting.pre_acceptance_estimate_trace
    ADD COLUMN service_product text,
    ADD COLUMN detail_recorded boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT pre_acceptance_estimate_trace_service_product_present
        CHECK (service_product IS NULL OR btrim(service_product) <> ''),
    -- 补记之前的行没有产品可记；有产品而标着没补记，是写口把两种行混了。
    ADD CONSTRAINT pre_acceptance_estimate_trace_service_product_recorded_with_detail
        CHECK (detail_recorded OR service_product IS NULL);

ALTER TABLE settlement_accounting.pre_acceptance_estimate_trace
    ALTER COLUMN detail_recorded DROP DEFAULT;

-- 读口按（租户，委托）取一票的各版估价、按试算时点排（决定二）；主键以控制请求标识收尾，前缀用不上。
CREATE INDEX pre_acceptance_estimate_trace_by_shipment_request
    ON settlement_accounting.pre_acceptance_estimate_trace (tenant_id, shipment_request_ref, trial_at);
