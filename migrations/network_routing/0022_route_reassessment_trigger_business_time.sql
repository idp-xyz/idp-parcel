-- 复核按业务时间排（ADR-0235）：每条复核记下所判触发的业务时间，较早的触发据此入账为「触发早于当前位置」、不判。
--
-- route_reassessment.trigger_occurred_at 是触发的业务时间，不是 reassessed_at 那个处理时钟——按处理时刻比，就又成了
-- 按到达顺序裁决。本迁移之前落的行没有这一格、无从补回，留空，不参与当前位置的比较；CHECK 以 NOT VALID 加上，只管
-- 此后写入的行。索引按判断键加业务时间，取「这件包裹最近一次已判控制触发」不扫全表。
--
-- route_reassessment_not_judged 加一格原因与比较所用的当前位置时刻：只在触发早于当前位置时有值，且必早于它。

ALTER TABLE network_routing.route_reassessment
    ADD COLUMN trigger_occurred_at timestamptz;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_trigger_occurred_at_present
        CHECK (trigger_occurred_at IS NOT NULL) NOT VALID;

CREATE INDEX route_reassessment_by_judgment_key_and_trigger_time
    ON network_routing.route_reassessment
       (tenant_id, customer_account_id, shipment_request_id,
        acceptance_baseline, declared_parcel_id, service_purpose, trigger_occurred_at);

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD COLUMN current_position_at timestamptz;

ALTER TABLE network_routing.route_reassessment_not_judged
    DROP CONSTRAINT route_reassessment_not_judged_reason_closed;

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED',
             'TRIGGER_PRECEDES_CURRENT_POSITION'));

-- reason 非空，等号两侧都不会是 NULL；时刻先后只在带了当前位置时比。
ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_position_iff_behind
        CHECK (
            (reason = 'TRIGGER_PRECEDES_CURRENT_POSITION') = (current_position_at IS NOT NULL)
            AND (current_position_at IS NULL OR trigger_occurred_at < current_position_at)
        );
