-- 收寄记录带身份冲突处置文档（ADR-0210 决定三；票 no-reception-identity/06）。只加一列与它的形状约束，不动既有各列与约束。
--
-- disposition_document：身份冲突处置文档（domain.IdentityDisposition 的 NID-1 形状，自带形状版本），只在处置版上有，与那一版同一
-- 行、同一笔事务写下，此后不改。同一次处置追加的各版带同一份文档。
--
-- **既有行留空，不回填。** 本迁移之前没有任何一条路径写处置版：身份冲突此前没有处置入口（ADR-0199 决定三「记身份冲突的不重核」、
-- ADR-0196「解除要等身份冲突的处置能力」），收寄册上的每一行都是首判、冲突版、重核版或续办版，没有一行是处置版。空即「不是处置
-- 版」，是真话。
--
-- 库面守与新旧无关的形状：文档是对象；只落在带收寄与控制的两格上（处置版是这两格之一）；处置版必定指名前版（它接续的是那条链此刻
-- 的当前有效版本）；处置版不记身份冲突（两种结果都把冲突解开）。文档内部的形状版本与各格由领域读回门守（ParseIdentityDisposition），
-- 库面不重写一遍。
--
-- 按包裹读关联的两面（FindOtherUnitIntakes、ParcelContainmentView）沿后继找处置版，每一步走 reception_predecessor_unique，不另建
-- 索引。

ALTER TABLE node_operations.reception
    ADD COLUMN disposition_document jsonb;

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_disposition_shaped
        CHECK (
            disposition_document IS NULL
            OR (jsonb_typeof(disposition_document) = 'object'
                AND kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION')
                AND predecessor_version IS NOT NULL
                AND NOT identity_conflict)
        );
