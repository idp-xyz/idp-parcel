-- 票 catalogue-read-pagination-rollout/12：治理登记册四册迁到 ADR-0144 之后，各册按「排序维 + 行标识」取页，每迁一册补一条
-- 与缺省序同形的索引（ADR-0144 决定七）。治理登记册没有租户维（ADR-0083 决定一），索引里也就没有租户那一段。

-- 生产权威区间：缺省序登记时间倒序，标识追加序 interval_id。
CREATE INDEX authority_interval_by_inserted
    ON pilot_governance.authority_interval (inserted_at DESC, interval_id DESC);

-- 暂停决定：缺省序发生时刻倒序，标识暂停标识。
CREATE INDEX suspension_decision_by_occurred
    ON pilot_governance.suspension_decision (occurred_at DESC, suspension_id DESC);

-- 恢复决定：缺省序决定时刻倒序，标识被恢复的暂停标识。
CREATE INDEX resumption_decision_by_decided
    ON pilot_governance.resumption_decision (decided_at DESC, suspension_id DESC);

-- 对象级接管：缺省序生效时刻倒序，标识区间四维、起点与「区间终点，开放区间取起点」。末段是表达式，须与读面落列逐字相同，
-- 规划器才认得这条索引。
CREATE INDEX takeover_record_by_effective
    ON pilot_governance.takeover_record
        (effective_at DESC, object_scope DESC, capability DESC, fact_kind DESC, authority DESC, from_at DESC,
         (COALESCE(to_at, from_at)) DESC);
