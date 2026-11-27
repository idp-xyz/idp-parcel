-- 预取的估算委托上申报了几件。放波次预取时按分到的全量申报；打包后那一个包裹的件数与它不同时
-- （拣不齐照货主的规则少发、拆出缺货部分，或一张单打成了几个包裹），要以货主名义提货物明细的资料修订，
-- 靠这一格知道当初报的是多少。本迁移之前预取的行没有这一格，为空。

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN prefetch_quantity bigint,
    ADD CONSTRAINT outbound_request_prefetch_quantity CHECK (prefetch_quantity IS NULL OR prefetch_quantity > 0);
