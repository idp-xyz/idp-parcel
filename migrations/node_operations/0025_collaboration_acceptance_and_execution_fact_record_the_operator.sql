-- 节点监管协作承接与执行事实两口把操作者记在记录上（ADR-0303 的待遇；ADR-0338 决定四；票 launch-wiring/04）。
--
-- 承接册多一列 decided_by，执行事实册多一列 recorded_by：作出这一次承接决定、登记这一条事实的操作者，取自认证信封，写法是
-- 「发行方#sub」。只进审计：不进内容摘要，重放、冲突与任何门都不读这一列。
--
-- 可空，不立 NOT NULL，也不回填：两口在本迁移之前没有生产入口，存量行（若有）只来自测试与脚本，没有可回填的来源。给了就不得是空白。

ALTER TABLE node_operations.collaboration_acceptance
    ADD COLUMN decided_by text;

ALTER TABLE node_operations.collaboration_acceptance
    ADD CONSTRAINT collaboration_acceptance_decided_by_not_blank
        CHECK (decided_by IS NULL OR btrim(decided_by) <> '');

ALTER TABLE node_operations.execution_fact
    ADD COLUMN recorded_by text;

ALTER TABLE node_operations.execution_fact
    ADD CONSTRAINT execution_fact_recorded_by_not_blank
        CHECK (recorded_by IS NULL OR btrim(recorded_by) <> '');
