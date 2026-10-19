-- 同一作业实物在同一节点已有当前收寄时，再次交付并入那一条收寄，不另起控制起点（ADR-0199 决定四，票 no-reception-source/04）。
--
-- kind 封闭集加「并入已有收寄」（MERGED_INTO_EXISTING_INTAKE）。它不是判断：不带收寄与控制（0001 的在场规则照旧拦住）、不签
-- 判断版本、不指名前版。并入不是后继——占了前版，那条链的当前有效版本就走到一行不是判断的记录上，（租户，前版）唯一也会把
-- 第二次并入拦成冲突。
--
-- merged_version：并入的那一版判断，只有并入行带、并入行必带。并入行必带来源文档：这一种记录就是为保全那次送站的来源才落的，
-- 并入行全是本迁移之后新落的，不存在「来源保全之前」的旧形状，库面可以无条件守住。不对 judgment_version 建外键：那一列上没有
-- 唯一约束，一个租户里不重名由签发器守（0008 的 reception_by_judgment_version 是普通索引）。
--
-- reception_judgments_by_unit_and_node：收寄编排按（租户，作业实物，节点）找各链的当前有效版本。形成与待识别按收寄上的实物与
-- 节点认，未收寄与待确认没有收寄，按来源文档认，一条 COALESCE 把两路并成一个键：ADR-0196 的冲突版来源文档里是后到那件，
-- 收寄上才是它接续的那件，intake 在前就认对了。只覆盖可被认出的判断：并入行不是判断，没有判断版本的旧行（来源保全之前的
-- 未收寄与待确认）也没存实物与节点，不进这本索引。查询里的表达式与谓词与这里逐字同形，规划器才认得它。

ALTER TABLE node_operations.reception
    DROP CONSTRAINT reception_kind_closed;

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_kind_closed
        CHECK (kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION', 'INTAKE_NOT_FORMED', 'RECEPTION_UNDECIDED',
                        'MERGED_INTO_EXISTING_INTAKE'));

ALTER TABLE node_operations.reception
    ADD COLUMN merged_version text;

-- 种类与并入版本同真同假；并入行的并入版本不空白、不带判断版本与前版、必带来源文档。merged_version 为 NULL 时第二段整体为
-- 真，不靠三值逻辑放行。
ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_merge_shaped
        CHECK ((kind = 'MERGED_INTO_EXISTING_INTAKE') = (merged_version IS NOT NULL)
           AND (merged_version IS NULL
                OR (btrim(merged_version) <> ''
                    AND judgment_version IS NULL
                    AND predecessor_version IS NULL
                    AND source_document IS NOT NULL)));

CREATE INDEX reception_judgments_by_unit_and_node
    ON node_operations.reception (tenant_id,
                                  (COALESCE(intake ->> 'unit', source_document ->> 'unit')),
                                  (COALESCE(intake ->> 'node', source_document ->> 'node')))
    WHERE kind <> 'MERGED_INTO_EXISTING_INTAKE'
      AND judgment_version IS NOT NULL;
