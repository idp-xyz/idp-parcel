-- 复核记下所据的控制依据与位置。实测变化不带来新位置：它抄最近一次已判控制触发的这两格再判一次，
-- 登记与作废同一条路。本迁移之前落的行没有这两格、无从补回，两列留空；「新行必带」用 NOT VALID，
-- 手法同 trigger_occurred_at（迁移 0022）——存量空列不回扫，此后写入的行两列都得有。
--
-- 两列同有同无。有值时控制依据只在能立成复核触发的封闭集里（线索立不成触发，不进这张册），位置非空白。
-- 这条对存量恒成立（两列都空），直接校验。

ALTER TABLE network_routing.route_reassessment
    ADD COLUMN trigger_control text,
    ADD COLUMN trigger_location text;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_trigger_control_position_paired
        CHECK (
            (trigger_control IS NULL) = (trigger_location IS NULL)
            AND (
                trigger_control IS NULL
                OR (
                    trigger_control IN (
                        'NODE_INTAKE_CONTROL', 'TRANSPORT_HANDOVER_CONTROL', 'OFFSITE_PICKUP_CONTROL')
                    AND btrim(trigger_location) <> ''
                )
            )
        );

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_trigger_control_position_present
        CHECK (trigger_control IS NOT NULL AND trigger_location IS NOT NULL) NOT VALID;

-- 实测变化到达时这一判断键上还没有可抄的控制位置（没有任何已判复核，或最近一次业务时刻上的位置定不下来），
-- 入账为「控制位置未定」、不判。等下一次控制触发再按那次的位置补判。
--
-- 这一格不带比较时刻：比的不是早于谁，是根本没有位置可抄。0022 之后各条「原因配时刻」的 CHECK 都是
-- 「恰是那一格原因才有那一列」；新原因两列都不占，那些 CHECK 放行。缺口与再判条件同此，可空。

ALTER TABLE network_routing.route_reassessment_not_judged
    DROP CONSTRAINT route_reassessment_not_judged_reason_closed;

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED',
             'TRIGGER_PRECEDES_CURRENT_POSITION', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE',
             'PLAN_APPLICABILITY_UNCLEAR', 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM',
             'CONTROL_POSITION_UNAVAILABLE'));
