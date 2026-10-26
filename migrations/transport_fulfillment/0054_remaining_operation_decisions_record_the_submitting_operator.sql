-- 其余运营决定口与派送补拍、按册重问把提交操作者记下来（ADR-0303 复核记录；票 demo-chain-gaps/180）。
--
-- 0051 让七口记下提交操作者；同一能力面上还有六口各形成一份决定记录，两口让派送发起执行器再问一次所有者。这里补上：
-- 订舱提交与取消、取消运输委托、履约段采用运输委托、容量预占与释放各在自己的决定记录上多一列；补拍与按册重问不记在任务上
-- （ADR-0303 决定五），另立一本只增不改的问询册，一次按下一行。
-- 写法与 0051 同一个「发行方#sub」，只进审计：重放、内容冲突与任何门都不读这些列，同一个决定换一位操作者重放，在册的
-- 仍是先到的那一位。
--
-- 可空，不立 NOT NULL，也不回填：存量行没有可回填的来源，拿别的列推一个人出来就是编造证据；NULL 读作「没记下是谁」。
-- 给了就不得是空白，并且只出现在它能属于的行上：取消人只在已取消的行上。存量行都是 NULL，新的 CHECK 对它们恒成立，
-- 直接校验。全部 CHECK 过 SQL 三值逻辑那一眼：新列可空，先 IS NULL；cancelled_at 可空，用之前先 IS NOT NULL。

-- 订舱：提交那一位与取消那一位各记各的。取消只改 cancelled_at 与 cancelled_by，不碰 submitted_by。
ALTER TABLE transport_fulfillment.booking_request
    ADD COLUMN submitted_by text,
    ADD COLUMN cancelled_by text;

ALTER TABLE transport_fulfillment.booking_request
    ADD CONSTRAINT booking_request_submitted_by_not_blank
        CHECK (submitted_by IS NULL OR btrim(submitted_by) <> ''),
    ADD CONSTRAINT booking_request_cancelled_by_only_when_cancelled
        CHECK (cancelled_by IS NULL OR (cancelled_at IS NOT NULL AND btrim(cancelled_by) <> ''));

-- 运输委托：取消那一位，与 submitted_by（0051）并列。开始是段采用带出的状态转换，不是一位操作者的决定，不记人。
ALTER TABLE transport_fulfillment.transport_commission
    ADD COLUMN cancelled_by text;

ALTER TABLE transport_fulfillment.transport_commission
    ADD CONSTRAINT transport_commission_cancelled_by_only_when_cancelled
        CHECK (cancelled_by IS NULL OR (cancelled_at IS NOT NULL AND btrim(cancelled_by) <> ''));

-- 履约段采用运输委托：采用的那一代各记各的；解除另有 withdrawn_by（0049），不在这一列上。
ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD COLUMN adopted_by text;

ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD CONSTRAINT segment_commission_adoption_adopted_by_not_blank
        CHECK (adopted_by IS NULL OR btrim(adopted_by) <> '');

-- 容量预占与释放：池按值换存时预占与释放明细整组删了再插回（CapacityPools.Replace），所以这两列随明细一起被写回，
-- 写回的值来自读回的记录，不来自哪一笔转换。
ALTER TABLE transport_fulfillment.capacity_reservation
    ADD COLUMN reserved_by text;

ALTER TABLE transport_fulfillment.capacity_reservation
    ADD CONSTRAINT capacity_reservation_reserved_by_not_blank
        CHECK (reserved_by IS NULL OR btrim(reserved_by) <> '');

ALTER TABLE transport_fulfillment.capacity_release
    ADD COLUMN released_by text;

ALTER TABLE transport_fulfillment.capacity_release
    ADD CONSTRAINT capacity_release_released_by_not_blank
        CHECK (released_by IS NULL OR btrim(released_by) <> '');

-- 派送问询册：谁在什么时候按了补拍或按册重问（ADR-0303 决定五）。
--
-- **不记在任务上，也不记在派送待形成册上。** 任务的内容出自所有者给的派送要求，形成它的是派送发起执行器，任务上不留建任务人；
-- 待形成册是随执行器最近一次答复改写的待办台账，一行只留最近一次，留不下「每一次是谁按的」，补拍的那一件也可能根本没有
-- 待形成行（一问就形成了）。所以另立一本只增不改的册：一次按下一行，补拍对单件、按册重问对批量，各自一行。
--
-- 只有经运营决定口按下的才有行：asked_by 不可空。执行器随交接信封发起的那一路、按册重问逐件交执行器的那一拍，都不是一位
-- 操作者的按下，不写这本册。重放不去重：同一件被两位操作者各补拍一次，就是两次按下，两行。
--
-- 补拍恰带段与对象、不带问法；按册重问恰带问法与件数上限、不带段与对象——两种按下的形状互斥，CHECK 钉住。
CREATE TABLE transport_fulfillment.delivery_dispatch_ask (
    ask_id      bigint      GENERATED ALWAYS AS IDENTITY,
    tenant_id   text        NOT NULL,
    kind        text        NOT NULL,
    asked_by    text        NOT NULL,
    -- 这一次按下的业务时间：补拍是请求里报的时刻，按册重问是重问那一刻。
    asked_at    timestamptz NOT NULL,
    segment_ref text,
    object_ref  text,
    scope       text,
    batch_limit integer,
    recorded_at timestamptz NOT NULL,

    CONSTRAINT delivery_dispatch_ask_pkey
        PRIMARY KEY (ask_id),

    CONSTRAINT delivery_dispatch_ask_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(asked_by) <> ''),

    CONSTRAINT delivery_dispatch_ask_shape
        CHECK (
            (kind = 'TRIGGER'
                AND segment_ref IS NOT NULL AND btrim(segment_ref) <> ''
                AND object_ref IS NOT NULL AND btrim(object_ref) <> ''
                AND scope IS NULL AND batch_limit IS NULL)
            OR (kind = 'REASK'
                AND segment_ref IS NULL AND object_ref IS NULL
                AND scope IN ('ALL', 'DUE') AND batch_limit IS NOT NULL AND batch_limit >= 1)
        )
);

-- 按租户由近及远查谁按过。
CREATE INDEX delivery_dispatch_ask_by_tenant
    ON transport_fulfillment.delivery_dispatch_ask (tenant_id, asked_at DESC, ask_id DESC);
