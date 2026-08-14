-- 运营结算余额与信用状况的登记：接受前财务控制读它们判断还能不能占。
--
-- **两张表都没有`冻结金额`与`已占用暴露`列。** 那两项由 funds_freeze 与 credit_exposure
-- 两本账当场求和得到（域内 HeldMinor / ExposedMinor）；在这里存第二份，只要有一次释放
-- 没同步过来，同一笔钱就会被冻第二次。列不存在，那种漂移就无处发生。
--
-- 键取（租户+责任法人+结算账户+币种）：CONTEXT 明写不同责任法人、结算账户与币种默认
-- 不共用余额，少一维就可能拿另一个作用域的钱来冻这一笔。两张表分立同 ADR-0047——预付
-- 冻结不与同一客户账期范围共用余额、额度，合成一张会让两本账在库里先合流。
--
-- 无行是`未登记`，视图据以报错而不是交回零值：一个各项为零的余额看起来像「查过了、
-- 就是没钱」，而实际是没人回答过。授信额度与逾期属实例半边，没有租户时两张表都是空的。

CREATE TABLE settlement_accounting.operational_balance (
    tenant_id       text        NOT NULL,
    legal_entity    text        NOT NULL,
    account_id      text        NOT NULL,
    currency        text        NOT NULL,

    posted_minor    bigint      NOT NULL,
    credit_minor    bigint      NOT NULL,
    unsettled_minor bigint      NOT NULL,
    as_of           timestamptz NOT NULL,
    inserted_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT operational_balance_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency),

    CONSTRAINT operational_balance_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
        ),

    -- 入账余额可负（确认费用高于冻结时差额形成欠款，CONTEXT 明禁截断），额度与已确认
    -- 未结不可负——它们是被扣减项，负值会把可用余额凭空撑大。
    CONSTRAINT operational_balance_deductions_not_negative
        CHECK (credit_minor >= 0 AND unsettled_minor >= 0)
);

CREATE TABLE settlement_accounting.credit_standing (
    tenant_id    text        NOT NULL,
    legal_entity text        NOT NULL,
    account_id   text        NOT NULL,
    currency     text        NOT NULL,

    limit_minor  bigint      NOT NULL,
    overdue      boolean     NOT NULL,
    as_of        timestamptz NOT NULL,
    inserted_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT credit_standing_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency),

    CONSTRAINT credit_standing_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
        ),

    CONSTRAINT credit_standing_limit_not_negative
        CHECK (limit_minor >= 0)
);
