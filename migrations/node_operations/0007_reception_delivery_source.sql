-- 收寄记录保全本次交付的来源文档；每一版判断带判断版本，追加的一版指名前版（ADR-0199 决定一、决定二）。
--
-- source_document：来源文档（domain.DeliverySource 的 NRS-1 形状，自带形状版本），与判断同一行、同一笔事务写下，此后不改。
-- **既有行不回填，留空。** 外部标识观察此前从未在册上留存：行里只有 NOC-1 内容摘要，那是哈希，还原不出号码；拒收行没存
-- 证据与交付方，待确认行连作业实物与节点都没存。拿行里残存的几格拼一份文档，就是给它补造一份从未保全过的来源。读面对空的
-- 那一格如实说「来源未保全」，重核与按号匹配都不碰这些行。
--
-- judgment_version：判断版本。形成与待识别两格就是 intake 里的收寄结果版本、不另签，库面守两处同值；既有的这两格回填
-- intake ->> 'version'——判断版本按定义就是它，回填是同一个值换一处写，不是推定。既有的未收寄与待确认两格从未签过版本，
-- 留空：此刻补签一个，就是在册上造出一版当时并不存在的判断。这些行因此不能被指名为前版，续办仅扫描待确认的那一票
-- （no-reception-source/04）要面对这一格。
--
-- predecessor_version：追加的一版所接续的那条判断链此刻的当前有效版本。（租户，前版）唯一守住「一版至多一个后继」，只覆盖
-- 带前版的行；当前有效版本沿这条边走到头，收寄库的 CurrentVersion 按它走。**既有的 ADR-0196 冲突版不回填前版**：它落下时
-- 没有指名接续的是哪一版，册上也没留下当时的选择——同一件先到实物收过多次时依的是哪一行，事后重算推不准。
--
-- **新行必带来源文档与判断版本，这一格库里不拦，由编排守。** 收寄表唯一的生产写方是 Receptions.Save，经它落册的只有收寄
-- 编排，编排每一支都写这两格，各支用例钉着。库面要拦就得按写入先后分新旧（NOT VALID 的 CHECK），可直接经 Save 或 SQL 落
-- 记录形状夹具的既有用例横跨多个包，收紧要连带改它们；理由同 0006 头注「INTAKE_FORMED 不带核对态这一格库里不拦」。库面守的
-- 是与新旧无关的形状：判断版本与收寄结果版本同值、带前版的行自己有判断版本且前版不是自己、来源文档是对象、版本不为空白。

ALTER TABLE node_operations.reception
    ADD COLUMN source_document jsonb;

ALTER TABLE node_operations.reception
    ADD COLUMN judgment_version text;

ALTER TABLE node_operations.reception
    ADD COLUMN predecessor_version text;

UPDATE node_operations.reception
   SET judgment_version = intake ->> 'version'
 WHERE intake IS NOT NULL;

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_versions_not_blank
        CHECK ((judgment_version IS NULL OR btrim(judgment_version) <> '')
           AND (predecessor_version IS NULL OR btrim(predecessor_version) <> ''));

-- intake 里没有版本却带判断版本也拦：用 IS NOT DISTINCT FROM，免得 NULL 让这条按三值逻辑放行。
ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_judgment_version_is_intake_version
        CHECK (judgment_version IS NULL OR intake IS NULL OR judgment_version IS NOT DISTINCT FROM (intake ->> 'version'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_predecessor_shaped
        CHECK (predecessor_version IS NULL OR (judgment_version IS NOT NULL AND predecessor_version <> judgment_version));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_source_document_shaped
        CHECK (source_document IS NULL OR jsonb_typeof(source_document) = 'object');

CREATE UNIQUE INDEX reception_predecessor_unique
    ON node_operations.reception (tenant_id, predecessor_version)
    WHERE predecessor_version IS NOT NULL;
