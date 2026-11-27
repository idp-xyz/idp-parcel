-- 成波规则的容量：一个波次除了最多几张出库请求，还可以限最多几行、最多几件。两格都是租户的取值，
-- 空着就是不限。行数按出库请求的预占行数算，件数按预占的基本单位数量算——一个拣货的人一趟拿不拿得完，
-- 看的是要走几个点、拿几件，不只是几张单。一张请求自己就超了的单独成一个波次：一张请求拆不开。

ALTER TABLE warehouse_fulfillment.wave_rule
    ADD COLUMN max_lines integer,
    ADD COLUMN max_units bigint,
    ADD CONSTRAINT wave_rule_max_lines_positive CHECK (max_lines IS NULL OR max_lines >= 1),
    ADD CONSTRAINT wave_rule_max_units_positive CHECK (max_units IS NULL OR max_units >= 1);
