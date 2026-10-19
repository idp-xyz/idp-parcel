-- 收寄记录的身份核对态（ADR-0199 决定五，票 no-reception-source/05）：待识别记录上，身份核对读口答复了（零候选、多候选或冲突）
-- 记 CHECKED，读口交回错误、没取得 PS 的答复记 UNCHECKED。两者的候选都可能是空数组，只有这一列把「查过了，查无此号」与
-- 「没问到」分开（ADR-0089「绝不交回空候选顶替」）。
--
-- 库面守形状，绕过适配器直接写库的一行也过这道门：取值封闭；只有 PENDING_IDENTIFICATION 带（关联收寄有了身份，拒收、待确认
-- 与并入没有可核对的身份）；UNCHECKED 的候选必为空数组、不记身份冲突、必带来源文档——没取得答复就没有候选可记，也就没有冲突；
-- 未核对行是为重核才落的，重核拿的就是来源文档里那一格号码，而未核对行全是本迁移之后新落的，不存在「来源保全之前」的旧形状。
-- 比较前先 COALESCE，免得 NULL 让整条 CHECK 按三值逻辑放行。PENDING_IDENTIFICATION 不带核对态这一格库里不拦：编排对每条待识别
-- 都写，回填之后既有行也都带；收紧成「必带」要连带改所有直接落待识别的测试夹具，留给需要它的那一票。身份冲突处置版里不带关联
-- 的那几版也是待识别行，记 CHECKED，与这几条约束相容。
--
-- **既有 PENDING_IDENTIFICATION 行回填 CHECKED**，这是推定，依据如下（取证时的 SHA 钉在这里，原因写在各自的提交说明里）：
--   - 36aff6e9（收寄编排初版）起，身份核对读口报错时编排答不落册的`收寄判断待确认`，返回在写收寄库之前；
--   - f70baa3a（票 operator-channel/29，ADR-0186 决定二）起改为交回 error、不形成答案，同样什么都不落——这就是票 operator-channel/29
--     记下的「零行」现象；到本迁移所在的票之前一直如此；
--   - 其余落待识别行的路都在读口答复之后：d783d4f8（ADR-0196 一码两物的冲突版）、18167998（票 no-reception-source/03 重核追加的
--     冲突版）、47329657（ADR-0210 身份冲突处置追加的不带关联那几版，接续的是读口答复过的冲突版）；
--   - 收寄表唯一的生产写方是 NO 的 postgres 适配器（Receptions.Save），没有别的路径写下 PENDING_IDENTIFICATION 行。
-- 于是每一条既有待识别行都是在取得 PS 答复之后写的，回填 CHECKED 说的是当时的真话。其余种类不回填。
--
-- reception_unchecked_pending：ParcelContainmentView 按租户找此刻在袋里的未核对待识别（ADR-0199 决定五）。只覆盖未核对行，行数
-- 就是 PS 读不动期间落下、还没重核续上的那几件。

ALTER TABLE node_operations.reception
    ADD COLUMN identity_check text;

UPDATE node_operations.reception
   SET identity_check = 'CHECKED'
 WHERE kind = 'PENDING_IDENTIFICATION';

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_identity_check_closed
        CHECK (identity_check IS NULL OR identity_check IN ('CHECKED', 'UNCHECKED'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_identity_check_pending_only
        CHECK (kind = 'PENDING_IDENTIFICATION' OR identity_check IS NULL);

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_unchecked_shaped
        CHECK (COALESCE(identity_check, '') <> 'UNCHECKED'
            OR (candidates = '[]'::jsonb AND NOT identity_conflict AND source_document IS NOT NULL));

CREATE INDEX reception_unchecked_pending
    ON node_operations.reception (tenant_id, (intake ->> 'unit'))
    WHERE identity_check = 'UNCHECKED';
