-- 委托聚合快照：以产生委托的来源身份为键，整份聚合存一列 jsonb。
--
-- 行模型照 RehydrateShipmentRequestSpec 设计：键、乐观版本与状态是列（键定位、
-- 版本挡并发、状态供巡检），其余全部进快照文档——文档的形状就是重建规格的形状，
-- 读回时逐字段经领域构造函数与 RehydrateShipmentRequest 的校验，不在 SQL 里摊平
-- 三层嵌套（历史版本×历史任务×逐版本画像）再手工拼回。
--
-- revision 是框架合同的严格递增一：Insert 写 1，每次 Save 在 WHERE 里带读出时的
-- 版本并加一。UPDATE 不带 revision 条件就是最后写入者赢，而两个并发决定都以为
-- 自己落库了正是 ADR-0031 要防的事。

CREATE TABLE parcel_shipment.shipment_request (
    tenant_id           text        NOT NULL,
    customer_account_id text        NOT NULL,
    source              text        NOT NULL,
    source_request_key  text        NOT NULL,

    shipment_request_id text        NOT NULL,
    revision            bigint      NOT NULL,
    state               smallint    NOT NULL,
    snapshot            jsonb       NOT NULL,
    submitted_at        timestamptz NOT NULL,
    saved_at            timestamptz NOT NULL DEFAULT now(),

    -- 主键即完整来源身份：重放按它返回原委托而不是再建一份。
    CONSTRAINT shipment_request_pkey
        PRIMARY KEY (tenant_id, customer_account_id, source, source_request_key),

    -- 委托身份在租户内唯一。跨租户不设唯一：两个租户各有一个同号委托是隔离的
    -- 正常形态，不是冲突。
    CONSTRAINT shipment_request_id_unique
        UNIQUE (tenant_id, shipment_request_id),

    CONSTRAINT shipment_request_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(source) <> ''
            AND btrim(source_request_key) <> ''
        ),
    CONSTRAINT shipment_request_id_not_blank
        CHECK (btrim(shipment_request_id) <> ''),
    -- 重建规格要求版本从 1 起：一行 revision 0 是没写完的行，不是一份聚合。
    CONSTRAINT shipment_request_revision_positive
        CHECK (revision >= 1),
    CONSTRAINT shipment_request_state_positive
        CHECK (state > 0)
);
