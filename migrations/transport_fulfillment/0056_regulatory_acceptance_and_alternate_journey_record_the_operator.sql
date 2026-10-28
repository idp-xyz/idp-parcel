-- 监管运输处置承接与替代/退运旅程启动两口把提交操作者记在决定记录上（ADR-0303 的待遇；ADR-0337 决定二；票 docs-consistency/code/43）。
--
-- 两口各在自己的记录上多一列操作证据：作出这一次承接决定、启动这一条旅程的操作者，取自操作者信封，写法与 0051 各列同一个
-- 「发行方#sub」。只进审计：重放、内容冲突与任何门都不读这一列。
--
-- 可空，不立 NOT NULL，也不回填：两口在本迁移之前没有生产入口，存量行（若有）只来自测试与脚本，没有可回填的来源。给了就不得
-- 是空白（与 0051 同一条）。新列可空，CHECK 先 IS NULL。

ALTER TABLE transport_fulfillment.disposition_acceptance
    ADD COLUMN decided_by text;

ALTER TABLE transport_fulfillment.disposition_acceptance
    ADD CONSTRAINT disposition_acceptance_decided_by_not_blank
        CHECK (decided_by IS NULL OR btrim(decided_by) <> '');

ALTER TABLE transport_fulfillment.alternate_journey
    ADD COLUMN started_by text;

ALTER TABLE transport_fulfillment.alternate_journey
    ADD CONSTRAINT alternate_journey_started_by_not_blank
        CHECK (started_by IS NULL OR btrim(started_by) <> '');
