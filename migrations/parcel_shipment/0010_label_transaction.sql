-- 面单交易聚合快照（ADR-0084 决定八，票 admin-skeleton-closure-batch/08）。
--
-- 键是（租户 + 面单交易标识）而不是来源身份：面单交易是独立聚合，覆盖包裹可以跨委托
-- （ADR-0084 决定一），拿委托的来源身份作键会让一笔跨委托交易无处安放。
--
-- 行模型照 RehydrateLabelTransactionSpec 设计，与 shipment_request 同纹样：键、乐观版本
-- 与状态是列（键定位、版本挡并发、状态供巡检与队列过滤），其余全部进快照文档——双层结果
-- （交易级 + 逐包裹）与追加式后续动作清单在 SQL 里摊平会变成三张表，而「同时保存整笔交易
-- 结果和各包裹结果」这条硬句要求它们同一次写入落地，一列 jsonb 天然给到这一点。读回时
-- 逐字段经领域构造函数与 RehydrateLabelTransaction 的校验。
--
-- **刻意没有 finalized 列。** ADR-0084 决定四说投影列「允许存在」而不是要求：定案是
-- state 落在三个结果格里这件事的另一种说法，而 state 已经在列上，state BETWEEN 4 AND 6
-- 就是它。多存一列等于把同一个事实写两处，而那正是决定四否决「定案入列存储」的理由。
--
-- state 取值镜像领域 LabelTransactionState 的封闭集合：
-- 1 = ESTABLISHED，2 = SUBMITTED_TO_CHANNEL，3 = RESULT_UNCERTAIN，
-- 4 = SUCCEEDED，5 = PARTIALLY_SUCCEEDED，6 = FAILED。
--
-- 渠道墙未降前本表零行是设计而不是欠账：写入方是渠道适配器，而首发基线明写「独立面单
-- 渠道服务不进入首发生产」。表与不变式先立起来，墙降那天写入方对着的不是一张裸表。

CREATE TABLE parcel_shipment.label_transaction (
    tenant_id            text        NOT NULL,
    label_transaction_id text        NOT NULL,

    revision             bigint      NOT NULL,
    state                smallint    NOT NULL,
    snapshot             jsonb       NOT NULL,
    established_at       timestamptz NOT NULL,
    saved_at             timestamptz NOT NULL DEFAULT now(),

    -- 主键即聚合键。租户维照 ADR-0003 的隔离边界，跨租户同号不是冲突。
    CONSTRAINT label_transaction_pkey
        PRIMARY KEY (tenant_id, label_transaction_id),

    CONSTRAINT label_transaction_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(label_transaction_id) <> ''
        ),
    -- 重建规格要求版本从 1 起：一行 revision 0 是没写完的行，不是一份聚合。
    CONSTRAINT label_transaction_revision_positive
        CHECK (revision >= 1),
    CONSTRAINT label_transaction_state_known
        CHECK (state BETWEEN 1 AND 6)
);

-- 读面签名是（租户, limit）：客户账户不是交易维度——覆盖包裹可以跨委托，按客户过滤会
-- 把一笔跨客户的交易归给其中一个客户（ADR-0084 决定七）。按建立时间倒序出册。
CREATE INDEX label_transaction_by_tenant
    ON parcel_shipment.label_transaction (tenant_id, established_at DESC);
