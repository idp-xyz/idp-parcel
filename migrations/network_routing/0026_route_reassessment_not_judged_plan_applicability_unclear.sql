-- 复核里的「适用性看不清」入账未判（ADR-0250 决定一）：原计划选中候选的限制状态未知时不回滚重投，答「适用性看不清」
-- （PLAN_APPLICABILITY_UNCLEAR），计划与适用性都不动。
--
-- route_reassessment_not_judged 加一格原因与两列：取数侧交回的缺口（evidence_gap）与再判条件（reassessment_condition）。
-- 这几种来源的补法各不相同——等关务口岸路径目录登记、等部署接上关务判断口、受理基线缺的端点国家永远补不上——只记原因，
-- 运维与续办就得回头再问一次取数侧，才认得出哪几件包裹在等哪一项登记。
--
-- 这一格原因必带两列，其余原因可空。两列成对：缺口没有出口是一句续不上的否定，出口没有缺口说不出在等什么（AT-NR-018）。
--
-- 已有的行原因都不是新这一格、两列为空，新加的 CHECK 对它们恒成立，直接校验，不必 NOT VALID。

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD COLUMN evidence_gap           text,
    ADD COLUMN reassessment_condition text;

ALTER TABLE network_routing.route_reassessment_not_judged
    DROP CONSTRAINT route_reassessment_not_judged_reason_closed;

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_reason_closed
        CHECK (reason IN
            ('NO_ROUTING_HISTORY', 'REASSESS_EVIDENCE_NOT_CONFIGURED', 'FREEZE_FORM_UNCONFIGURED',
             'TRIGGER_PRECEDES_CURRENT_POSITION', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE',
             'PLAN_APPLICABILITY_UNCLEAR'));

ALTER TABLE network_routing.route_reassessment_not_judged
    ADD CONSTRAINT route_reassessment_not_judged_gap_paired
        CHECK (
            (evidence_gap IS NULL) = (reassessment_condition IS NULL)
            AND (evidence_gap IS NULL OR (btrim(evidence_gap) <> '' AND btrim(reassessment_condition) <> ''))
            AND (reason <> 'PLAN_APPLICABILITY_UNCLEAR' OR evidence_gap IS NOT NULL)
        );
