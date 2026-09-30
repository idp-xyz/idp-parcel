-- 服务日历的内容列（ADR-0175）。身份与有效区间已在 0008。
-- 三格都可空：空是没登记这一格，与登记了 0 不是一回事。不写默认时长，也不种租户行。

ALTER TABLE network_routing.service_calendar_version
    ADD COLUMN cutoff_local_minute integer,
    ADD COLUMN processing_minutes integer,
    ADD COLUMN buffer_minutes integer;

ALTER TABLE network_routing.service_calendar_version
    ADD CONSTRAINT service_calendar_version_cutoff_local
        CHECK (cutoff_local_minute IS NULL OR cutoff_local_minute BETWEEN 0 AND 1439),
    ADD CONSTRAINT service_calendar_version_processing
        CHECK (processing_minutes IS NULL OR processing_minutes >= 0),
    ADD CONSTRAINT service_calendar_version_buffer
        CHECK (buffer_minutes IS NULL OR buffer_minutes >= 0);
