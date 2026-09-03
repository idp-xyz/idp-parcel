-- `面单继续尝试决定`登记册（ADR-0084 决定六留待的那一册，票 label-channel/10）。
--
-- 键是（租户 + 包裹），不是面单交易也不是委托：CONTEXT 把决定定义为「针对明确包裹当前完整面单
-- 服务范围形成」，一个包裹可关联多笔重试、替代、作废或换单交易，挂交易上会让同一个包裹在不同
-- 交易下各有一套关闭状态；覆盖包裹又可以跨委托（ADR-0084 决定一），挂委托上一册跨委托的决定
-- 无处安放。
--
-- 行模型照 label_transaction 同纹样：键与乐观版本是列，决定链整份进快照文档，读回时逐字段经
-- 领域构造函数与 RehydrateContinuedAttemptRegister 的校验。
--
-- **刻意没有判断列，也没有「当前关闭在不在」的投影列。** CONTEXT 把包裹级继续尝试判断定义为
-- 「只由有效的关闭、重开决定及当前有效终局结果派生」，存一列就有了第二个来源，而终局有效性
-- 后来变化时那一列不会跟着变（CONTEXT 明写这种情形要重新派生）。读面要判断，就读回整册现算。
--
-- 也没有 state 列：登记册没有生命周期状态，它是一条只增的决定链；空册与非空册同样合法——空册
-- 正是`开放`那一格的常见来源，而不是「还没建」。
--
-- 写入方未立前本表零行是设计不是欠账：形成关闭或重开决定的命令口要先过 party-commercial 的
-- 授权校验（CONTEXT「形成关闭或重开决定时仍须重新校验当前角色与客户授权」），属另一张票。

CREATE TABLE parcel_shipment.continued_attempt_register (
    tenant_id  text        NOT NULL,
    parcel_id  text        NOT NULL,

    revision   bigint      NOT NULL,
    snapshot   jsonb       NOT NULL,
    opened_at  timestamptz NOT NULL DEFAULT now(),
    saved_at   timestamptz NOT NULL DEFAULT now(),

    -- 主键即聚合键。租户维照 ADR-0003 的隔离边界，跨租户同号不是冲突。
    CONSTRAINT continued_attempt_register_pkey
        PRIMARY KEY (tenant_id, parcel_id),

    CONSTRAINT continued_attempt_register_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_id) <> ''
        ),
    -- 重建规格要求版本从 1 起：一行 revision 0 是没写完的行，不是一份聚合。
    CONSTRAINT continued_attempt_register_revision_positive
        CHECK (revision >= 1)
);
