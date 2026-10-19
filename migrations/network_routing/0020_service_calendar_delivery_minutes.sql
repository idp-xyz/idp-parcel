-- 服务日历的派送时长（ADR-0217）：交付节点日历上的一格，登了，计划在这个节点之后带一段交付段，窗口是
-- 离开交付节点的时刻加上它。与 0015 的截单、处理时长、衔接缓冲同一约定：可空，空是没登记这一格，登记了 0
-- 才是零分钟；不写默认时长，也不种租户行。只登在节点日历上才有读者，连接与线路日历上的这一格不被读。

ALTER TABLE network_routing.service_calendar_version
    ADD COLUMN delivery_minutes integer;

ALTER TABLE network_routing.service_calendar_version
    ADD CONSTRAINT service_calendar_version_delivery
        CHECK (delivery_minutes IS NULL OR delivery_minutes >= 0);
