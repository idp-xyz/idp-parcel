-- 复核的第三道业务时间下限（ADR-0292）：触发的业务时间早于起点计划的生效时刻，且判出失效或改善替代时，入账为「触发早于计划
-- 生效」、不判。授权角色的决定形成的计划从决定的时刻起生效（ADR-0284 决定四），业务时间落在原计划失效与这一决定之间、处理却在
-- 决定之后的触发，离场与接班计划的生效都早于这份计划的生效，离场门必拒。
--
-- route_reassessment_not_judged 加一格原因与比较所用的计划生效时刻：只在这一格原因下有值，且触发不晚于它；审计要答得出
-- 早于的是哪一刻，与「早于委托接受」那一格记 shipment_accepted_at、「早于当前位置」那一格记 current_position_at 同一个道理。三格
-- 各记各的列，不合用一列：比的是三样东西。
--
-- 已有的行原因都不是新这一格、plan_effective_at 为空，等号两侧同为假，新加的 CHECK 对它们恒成立，直接校验，不必 NOT VALID。

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD COLUMN plan_effective_at timestamptz;

ALTER TABLE network_routing.route_reassessment_not_judged
    DROP CONSTRAINT route_reassessment_not_judged_reason_closed;

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED',
             'TRIGGER_PRECEDES_CURRENT_POSITION', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE',
             'PLAN_APPLICABILITY_UNCLEAR', 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM'));

-- 先后用 <= 而不用 <，理由同 0024：触发的业务时间读自交接册、收寄采用册的 timestamptz 列，只到微秒；计划的生效时刻在适用性
-- 没有登记行时读自计划文档里的 JSON 时间串，带纳秒。编排按纳秒比判早于，落库时生效时刻截到微秒、两格相等，用 < 就写不进，
-- 原因落不了库、整笔回滚，重投到 ABANDONED。<= 照样拒掉晚于生效的写入。
ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_plan_effective_iff_not_after
        CHECK (
            (reason = 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM') = (plan_effective_at IS NOT NULL)
            AND (plan_effective_at IS NULL OR trigger_occurred_at <= plan_effective_at)
        );
