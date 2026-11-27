-- 波次按库区派工：一个波次里某个库区的格派给一个人拣。没单独派的库区照整个波次的指派（wave.assignee）走。
-- 一个波次的一个库区至多一行；取消这个库区的指派就是删掉这一行。库区认的是任务所在库位此刻所属的库区。

CREATE TABLE warehouse_fulfillment.wave_zone_assignee (
    tenant_id   text        NOT NULL,
    wave_id     text        NOT NULL,
    zone_code   text        NOT NULL,
    assignee    text        NOT NULL,
    assigned_by text        NOT NULL,
    assigned_at timestamptz NOT NULL,

    CONSTRAINT wave_zone_assignee_pkey PRIMARY KEY (tenant_id, wave_id, zone_code),
    CONSTRAINT wave_zone_assignee_wave_fkey
        FOREIGN KEY (tenant_id, wave_id) REFERENCES warehouse_fulfillment.wave (tenant_id, wave_id),
    CONSTRAINT wave_zone_assignee_not_blank CHECK (
        btrim(zone_code) <> '' AND btrim(assignee) <> '' AND btrim(assigned_by) <> ''
    )
);
