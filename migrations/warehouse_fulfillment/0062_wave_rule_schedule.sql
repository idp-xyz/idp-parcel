-- 成波规则的时点：租户可以给一条规则登「每天几点放」，按仓的业务时区，写成 HH:MM。空着是只由人点「按规则成波」。
-- 产品里没有定时器：到点的规则由部署方的定时任务调「放到点的规则」那一口来放，口每次只放此刻已经到点、还没办过的时点。
--
-- wave_rule_slot 记一个时点办过了：某条规则在某个业务日的某个时点，一行。同一个时点只办一次——重复调不会放两遍，
-- 到点时没有可放的也算办过，不然之后才受理的单会在下一次调用时被这个早已过去的时点放走。

ALTER TABLE warehouse_fulfillment.wave_rule
    ADD COLUMN release_times text[] NOT NULL DEFAULT '{}';

CREATE TABLE warehouse_fulfillment.wave_rule_slot (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    rule_code      text        NOT NULL,
    slot_date      date        NOT NULL,
    slot_time      text        NOT NULL,
    run_id         text        NOT NULL,
    -- 办的结果：放出了波次、没有可放的，或同一个执行号之前已经成过。办之前先占下这一行，结果是空的。
    outcome        text,
    operator_id    text        NOT NULL,
    settled_at     timestamptz NOT NULL,

    CONSTRAINT wave_rule_slot_pkey PRIMARY KEY (tenant_id, warehouse_code, rule_code, slot_date, slot_time),
    CONSTRAINT wave_rule_slot_time CHECK (slot_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')
);
