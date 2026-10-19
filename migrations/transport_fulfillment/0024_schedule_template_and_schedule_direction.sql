-- 周期安排登记册；具体班次改以 NR 连接代码加起讫节点为方向身份，并按版本登记（票 demo-chain-gaps/16，ADR-0201）。
--
-- 0004 的班次行只有一格自由串方向。自由串与计划履约段比对不了，本迁移把它退役：方向改成连接代码、起点节点、
-- 终点节点三格，按 NR 的拼写原样保存。那一格从未有过生产写入口，表里应当没有行；有行就拒绝迁移，不替任何人
-- 把一个自由串猜成连接代码——那一行是谁登的、指哪条连接，只有登它的一方答得出。
--
-- 具体班次按（租户，班次，版本）登记，只插不改：同一班次异内容再登是下一版，当前版是版本号最大的那一行。
-- 版本号由编排按当前版加一给出，主键挡住两方同时落同一个版本号。来历两种：由周期安排按运行日期物化（带
-- 周期安排代码、物化时那一版与运行日期），或临时班次（三格全空）——两种的完备性由 CHECK 逐格守。
--
-- 周期安排同样按（租户，代码，版本）只插不改。正文是 ADR-0201 决定三的那一种文法：星期集合、当地出发分钟、
-- 历时分钟、IANA 时区、方向、服务范围与运行日期的生效区间（终点不含，可空）。时区名对不对由领域构造门核，
-- 库上不复制一份时区表。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.transport_schedule) THEN
        RAISE EXCEPTION USING MESSAGE =
            'transport_schedule 已有行，不把自由串方向猜成连接代码与起讫节点：请导出这些行备查，由登记它们的一方'
            || '按连接代码与起讫节点重新登记后清空本表，再重跑本迁移。';
    END IF;
END
$$;

DROP TABLE transport_fulfillment.transport_schedule;

CREATE TABLE transport_fulfillment.transport_schedule (
    tenant_id         text        NOT NULL,
    schedule_id       text        NOT NULL,
    schedule_version  integer     NOT NULL,

    connection_code   text        NOT NULL,
    from_node         text        NOT NULL,
    to_node           text        NOT NULL,
    departs_at        timestamptz NOT NULL,
    arrives_at        timestamptz NOT NULL,
    service_scope_ref text        NOT NULL,
    origin            text        NOT NULL,
    template_code     text,
    template_version  integer,
    running_date      date,
    content_digest    text        NOT NULL,
    recorded_at       timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT transport_schedule_pkey
        PRIMARY KEY (tenant_id, schedule_id, schedule_version),

    CONSTRAINT transport_schedule_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(schedule_id) <> ''
            AND btrim(connection_code) <> ''
            AND btrim(from_node) <> ''
            AND btrim(to_node) <> ''
            AND btrim(service_scope_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT transport_schedule_version_positive
        CHECK (schedule_version >= 1),

    CONSTRAINT transport_schedule_direction_distinct
        CHECK (from_node <> to_node),

    CONSTRAINT transport_schedule_arrives_after_departure
        CHECK (arrives_at > departs_at),

    CONSTRAINT transport_schedule_origin_closed
        CHECK (origin IN ('TEMPLATE', 'AD_HOC')),

    CONSTRAINT transport_schedule_origin_complete
        CHECK (
            (
                origin = 'TEMPLATE'
                AND template_code IS NOT NULL
                AND btrim(template_code) <> ''
                AND template_version IS NOT NULL
                AND template_version >= 1
                AND running_date IS NOT NULL
            )
            OR (
                origin = 'AD_HOC'
                AND template_code IS NULL
                AND template_version IS NULL
                AND running_date IS NULL
            )
        )
);

-- 候选按（租户，连接）取、按出发时刻筛。
CREATE INDEX transport_schedule_connection_departure
    ON transport_fulfillment.transport_schedule (tenant_id, connection_code, departs_at);

CREATE TABLE transport_fulfillment.schedule_template (
    tenant_id              text        NOT NULL,
    template_code          text        NOT NULL,
    template_version       integer     NOT NULL,

    connection_code        text        NOT NULL,
    from_node              text        NOT NULL,
    to_node                text        NOT NULL,
    weekdays               text[]      NOT NULL,
    departure_local_minute integer     NOT NULL,
    duration_minutes       bigint      NOT NULL,
    time_zone              text        NOT NULL,
    service_scope_ref      text        NOT NULL,
    effective_from         date        NOT NULL,
    effective_until        date,
    content_digest         text        NOT NULL,
    recorded_at            timestamptz NOT NULL,
    inserted_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT schedule_template_pkey
        PRIMARY KEY (tenant_id, template_code, template_version),

    CONSTRAINT schedule_template_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(template_code) <> ''
            AND btrim(connection_code) <> ''
            AND btrim(from_node) <> ''
            AND btrim(to_node) <> ''
            AND btrim(time_zone) <> ''
            AND btrim(service_scope_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    CONSTRAINT schedule_template_version_positive
        CHECK (template_version >= 1),

    CONSTRAINT schedule_template_direction_distinct
        CHECK (from_node <> to_node),

    -- 星期是封闭词集合；含 NULL 元素时 <@ 不成立，空集合由 cardinality 挡。
    CONSTRAINT schedule_template_weekdays_closed
        CHECK (
            cardinality(weekdays) > 0
            AND weekdays <@ ARRAY['MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT', 'SUN']::text[]
        ),

    CONSTRAINT schedule_template_departure_minute_of_day
        CHECK (departure_local_minute >= 0 AND departure_local_minute < 1440),

    CONSTRAINT schedule_template_duration_positive
        CHECK (duration_minutes > 0),

    CONSTRAINT schedule_template_effective_interval_ordered
        CHECK (effective_until IS NULL OR effective_until > effective_from)
);

CREATE INDEX schedule_template_connection
    ON transport_fulfillment.schedule_template (tenant_id, connection_code);
