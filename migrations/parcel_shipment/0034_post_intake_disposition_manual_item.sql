-- 待人工处理项（ADR-0341；票 launch-wiring/05）。收寄后服务处置决定向对象所有者提出的范围化请求里，系统今天不替所有者执行的几项，与决定同笔落成一行
-- 待人工处理项；运营人员在系统里逐项登记处理结果，登记即这一项的承接结果。
--
-- 一、post_intake_disposition_manual_item：一项一行，主键（租户，包裹，决定版本，所有者，动作）；外键指向决定链上那一版，所以项只能挂在已形成的决定上。
-- 决定链只追加，项也只追加：后来的决定取代了这一版，这一行照旧在册——包裹实物上被扣住这件事不因链尾换了而自行消失。
--
-- 二、post_intake_disposition_manual_result：每项至多一份结果（主键同项），外键指向项。结果封闭两值（已处理、无法处理），说明与操作者必填；不改不删。
-- 项与结果分两张表：结果的在场与否就是「待人工处理」与「已承接」的区别，不用状态列，状态列会被覆盖。
--
-- 所有者与动作的封闭集抄自 domain 的请求表；「系统执行的动作不成项」由编排保证，库里只钉封闭集。所有列都 NOT NULL，CHECK 里只有封闭集合与非空白判断，没有可空列上的三值逻辑。

CREATE TABLE parcel_shipment.post_intake_disposition_manual_item (
    tenant_id        text        NOT NULL,
    parcel_id        text        NOT NULL,
    decision_version integer     NOT NULL,
    owner            text        NOT NULL,
    action           text        NOT NULL,
    created_at       timestamptz NOT NULL,

    CONSTRAINT post_intake_disposition_manual_item_pkey
        PRIMARY KEY (tenant_id, parcel_id, decision_version, owner, action),

    CONSTRAINT post_intake_disposition_manual_item_decision_fkey
        FOREIGN KEY (tenant_id, parcel_id, decision_version)
        REFERENCES parcel_shipment.post_intake_disposition_decision (tenant_id, parcel_id, version),

    CONSTRAINT post_intake_disposition_manual_item_owner_closed
        CHECK (owner IN ('NETWORK_ROUTING', 'NODE_OPERATIONS', 'TRANSPORT_FULFILLMENT')),

    CONSTRAINT post_intake_disposition_manual_item_action_closed
        CHECK (action IN ('HOLD_AT_CONTROL', 'HOLD_ONWARD_MOVEMENT', 'REPLAN_TO_ALTERNATE_DESTINATION',
                          'START_ALTERNATE_JOURNEY', 'PLAN_RETURN_ROUTE', 'LOAD_FOR_RETURN',
                          'START_RETURN_JOURNEY', 'RELEASE_UNCONSUMED_PLAN')),

    CONSTRAINT post_intake_disposition_manual_item_shape
        CHECK (decision_version >= 1 AND btrim(tenant_id) <> '' AND btrim(parcel_id) <> '')
);

-- 运营查阅「待人工处理」按租户取最早的项；结果在另一张表，清单靠反连接，这条索引给按租户扫的那一半。
CREATE INDEX post_intake_disposition_manual_item_by_tenant
    ON parcel_shipment.post_intake_disposition_manual_item (tenant_id, created_at);

CREATE TABLE parcel_shipment.post_intake_disposition_manual_result (
    tenant_id        text        NOT NULL,
    parcel_id        text        NOT NULL,
    decision_version integer     NOT NULL,
    owner            text        NOT NULL,
    action           text        NOT NULL,
    outcome          text        NOT NULL,
    note             text        NOT NULL,
    handled_by       text        NOT NULL,
    handled_at       timestamptz NOT NULL,

    CONSTRAINT post_intake_disposition_manual_result_pkey
        PRIMARY KEY (tenant_id, parcel_id, decision_version, owner, action),

    CONSTRAINT post_intake_disposition_manual_result_item_fkey
        FOREIGN KEY (tenant_id, parcel_id, decision_version, owner, action)
        REFERENCES parcel_shipment.post_intake_disposition_manual_item (tenant_id, parcel_id, decision_version, owner, action),

    CONSTRAINT post_intake_disposition_manual_result_outcome_closed
        CHECK (outcome IN ('HANDLED', 'UNABLE')),

    CONSTRAINT post_intake_disposition_manual_result_not_blank
        CHECK (btrim(note) <> '' AND btrim(handled_by) <> '')
);
