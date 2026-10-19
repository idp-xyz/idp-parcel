-- BUY 触发册每行加「费用项目」一格（ADR-0208）。发生项已形成那一路从这里取费用项目：发生项与供应商协议上都没有它，
-- 结算也不拿原因或价卡费目顶上。
--
-- 可空而不是补默认：0027 时代登的行没有这一格，那一路对它答未配置——补一个默认费用项目就是替租户填值。ADR-0164
-- 那条由调用方交齐三件引用的触发面不读这一列。

ALTER TABLE settlement_accounting.buy_evaluation_trigger
    ADD COLUMN fee_item_ref text;

ALTER TABLE settlement_accounting.buy_evaluation_trigger
    ADD CONSTRAINT buy_evaluation_trigger_fee_item_not_blank
        CHECK (fee_item_ref IS NULL OR btrim(fee_item_ref) <> '');
