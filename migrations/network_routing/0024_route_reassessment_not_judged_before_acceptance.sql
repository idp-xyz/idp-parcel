-- 复核的业务时间下限（ADR-0279）：触发的业务时间早于这一判断键的委托接受时刻时，入账为「触发早于委托接受」、不判。
--
-- route_reassessment_not_judged 加一格原因与比较所用的委托接受时刻：只在这一格原因下有值，且触发必早于它；审计要答得出
-- 早于的是哪一刻，与「早于当前位置」那一格记 current_position_at 同一个道理。两格各记各的列，不合用一列：比的是两样
-- 东西，合成一列就读不出早于的是委托还是路由历史里的哪一次复核。
--
-- 已有的行原因都不是新这一格、shipment_accepted_at 为空，等号两侧同为假，新加的 CHECK 对它们恒成立，直接校验，不必 NOT VALID。

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD COLUMN shipment_accepted_at timestamptz;

ALTER TABLE network_routing.route_reassessment_not_judged
    DROP CONSTRAINT route_reassessment_not_judged_reason_closed;

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED',
             'TRIGGER_PRECEDES_CURRENT_POSITION', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE'));

-- reason 非空，等号两侧都不会是 NULL；时刻先后只在带了委托接受时刻时比。先后用 <= 而不用 <：编排按纳秒比，接受时刻取自
-- parcel-shipment、带纳秒，落进 timestamptz 时两格都只留到微秒；纳秒上早于、落在同一微秒里的那一次两格相等，用 < 就写不进，
-- 原因落不了库、整笔回滚，重投到 ABANDONED。<= 照样拒掉晚于接受的写入。
ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_acceptance_iff_before
        CHECK (
            (reason = 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE') = (shipment_accepted_at IS NOT NULL)
            AND (shipment_accepted_at IS NULL OR trigger_occurred_at <= shipment_accepted_at)
        );
