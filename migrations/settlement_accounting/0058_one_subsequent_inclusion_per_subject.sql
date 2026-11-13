-- 一笔后到费用、一笔调整各只能被纳入后续账期一次（UC-SA-003 步 7）。
--
-- 纳入册的主键是（租户，纳入标识）：换一个纳入标识把同一笔费用或同一笔调整再纳一次，主键拦不住——它会在后续账期里
-- 被算两遍。纳入口此前没有生产入口，这个口子没人走得到；入口接上的同一笔补上这两条。
--
-- 用 adjustment_id 是否为空分两种纳入：调整纳入的那一行也带着调整所挂的费用，所以费用那一条只管没有调整的行，否则
-- 一笔费用被纳入之后它的调整就纳不进来了。

CREATE UNIQUE INDEX subsequent_inclusion_one_per_late_charge
    ON settlement_accounting.subsequent_inclusion (tenant_id, charge_id)
    WHERE adjustment_id IS NULL;

CREATE UNIQUE INDEX subsequent_inclusion_one_per_adjustment
    ON settlement_accounting.subsequent_inclusion (tenant_id, adjustment_id)
    WHERE adjustment_id IS NOT NULL;
