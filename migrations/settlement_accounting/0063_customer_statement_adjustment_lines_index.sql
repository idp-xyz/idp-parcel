-- 找「还没出账的调整」时，对每笔候选调整去探对账单的调整行：
--   customer_statement.adjustment_lines @> jsonb_build_array(jsonb_build_object('adjustment', <调整标识>))
-- 这一列没有索引，对账单一多，每笔调整都把整张对账单扫一遍。
--
-- 用 jsonb_path_ops：这条探查只用包含判断（@>），这一类 GIN 就为它而设，比默认的 jsonb_ops 小也快。它不支持键存在判断（?），
-- 本表的读法里没有这一种。查询语义与结果不变，只多一条可走的路；不改任何列与行，也不种任何租户的行。

CREATE INDEX customer_statement_adjustment_lines_gin
    ON settlement_accounting.customer_statement
    USING gin (adjustment_lines jsonb_path_ops);
