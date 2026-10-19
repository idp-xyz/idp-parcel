-- 收寄记录的冲突来由（ADR-0211 决定一、二，票 no-reception-identity/07）：一版判断记身份冲突时，记下它从哪一格来——只因歧义信号
-- （PS 答恰一个候选、同时答出歧义信号）、多候选、一码两物。只有只因歧义信号那一格在信号消失之后经重核自动解开，其余只经身份冲突
-- 处置解开；这一列就是两者的分界。
--
-- 库面守与新旧无关的形状：取值封闭；不记身份冲突的行不带来由。记身份冲突的行**不**要求必带：
--
-- **既有冲突行留空，不回填。** 本迁移之前落下的冲突行，来由只能从候选数与当时的歧义信号推，而信号从不落册——恰一个候选的冲突行
-- 可能是只因歧义信号，也可能是一码两物（后到那件与先到那件追加的冲突版都只带一个候选）。推错一行，就把一桩一码两物的冲突交给
-- 自动解开。空即「来由未记」，这些行照旧只经身份冲突处置解开（ADR-0210 决定一）。
--
-- reception_recheckable_by_mark 按 ADR-0211 决定二重建：重核对象放进来由为只因歧义信号的冲突行。部分条件与重核按号找那一问
-- （Receptions.FindRecheckableByMark）的谓词逐字同形，规划器才认得出这本索引可用；改一边不改另一边，查询就悄悄退成全表扫。
-- 键照迁移 0008 取号码的 md5，理由见那一份的头注。

ALTER TABLE node_operations.reception
    ADD COLUMN identity_conflict_cause text;

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_identity_conflict_cause_closed
        CHECK (identity_conflict_cause IS NULL
            OR identity_conflict_cause IN ('AMBIGUITY_ONLY', 'MULTIPLE_CANDIDATES', 'ONE_MARK_ON_TWO_UNITS'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_identity_conflict_cause_conflict_only
        CHECK (identity_conflict_cause IS NULL OR identity_conflict);

DROP INDEX node_operations.reception_recheckable_by_mark;

CREATE INDEX reception_recheckable_by_mark
    ON node_operations.reception (tenant_id, md5(source_document ->> 'mark'))
    WHERE kind = 'PENDING_IDENTIFICATION'
      AND (NOT identity_conflict OR identity_conflict_cause = 'AMBIGUITY_ONLY')
      AND source_document IS NOT NULL;
