-- 对账周期登记册与到点截单的截止记录（ADR-0339 决定三）。
--
-- 对账周期：按（租户、结算账户）登一份切法——日截、周截（点名星期几，0 是星期日）、月截（点名 1–28，0 是月末）——与起算日。
-- 起算日只定第一个截止日从哪天之后算起。一户只登一份，同内容重登是重放，异内容不覆盖：改周期怎么接上已截的各期还没定。它是对账
-- 安排之下的第二本册：业务时区与截单时刻那一本按版次追加，周期的变更规则不同，分开登。账户不在账户册上由外键拒。不登任何租户的行。
CREATE TABLE settlement_accounting.statement_cycle (
    tenant_id         text        NOT NULL,
    account_id        text        NOT NULL,
    cycle_kind        text        NOT NULL,
    closing_weekday   smallint,
    closing_month_day smallint,
    since_date        date        NOT NULL,
    registered_at     timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT statement_cycle_pkey
        PRIMARY KEY (tenant_id, account_id),

    CONSTRAINT statement_cycle_account_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.settlement_account (tenant_id, account_id),

    CONSTRAINT statement_cycle_text_present
        CHECK (btrim(tenant_id) <> '' AND btrim(account_id) <> ''),

    CONSTRAINT statement_cycle_kind_shape
        CHECK (
            (cycle_kind = 'DAILY' AND closing_weekday IS NULL AND closing_month_day IS NULL)
            OR (cycle_kind = 'WEEKLY' AND closing_weekday BETWEEN 0 AND 6 AND closing_month_day IS NULL)
            OR (cycle_kind = 'MONTHLY' AND closing_weekday IS NULL AND closing_month_day BETWEEN 0 AND 28)
        )
);

-- 到点截单逐个截止日记一行：截了什么、答了什么。执行器从最后一行之后的截止日起截，所以一个截止日只截一次；本期没有费用也记
-- 一行（NO_CHARGE），否则那一天每拍都要重问一遍。截单停在依赖上不记行，下一拍再截。
CREATE TABLE settlement_accounting.statement_cycle_cut (
    tenant_id        text        NOT NULL,
    account_id       text        NOT NULL,
    closing_date     date        NOT NULL,
    outcome          text        NOT NULL,
    statement_number text,
    cut_at           timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT statement_cycle_cut_pkey
        PRIMARY KEY (tenant_id, account_id, closing_date),

    CONSTRAINT statement_cycle_cut_cycle_fkey
        FOREIGN KEY (tenant_id, account_id)
        REFERENCES settlement_accounting.statement_cycle (tenant_id, account_id),

    CONSTRAINT statement_cycle_cut_outcome_present
        CHECK (btrim(outcome) <> '')
);
