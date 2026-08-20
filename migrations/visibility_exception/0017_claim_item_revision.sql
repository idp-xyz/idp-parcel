-- 索赔行加修订列，承担丢更新防护。
--
-- 0004 的 claim_item 按（租户+批次+项）整行 UPSERT：两个并发事务各自 load → 领域校验
-- → Save 时，READ COMMITTED 下两边都能提交，后写者按自己的旧快照整行重写，前一个转换
-- （资格审核、撤回、复核、延期）就此消失，而两边都以为自己成功。领域门只在各自加载的
-- 旧快照上校验过，拦不住这种交错。
--
-- 修订列让「检查—递增」落在一条条件 UPDATE 里：适配器带着读出时的修订写回，命中零行
-- 即有人先落了一步。CHECK 在库内再守一遍列的取值域——绕过适配器的裸写同样进不来
-- （同 0004 对三判形状的办法）。
--
-- 默认值留着，不在建列后 DROP DEFAULT：存量行要靠它落到首版，而 0004 那批三判形状
-- CHECK 的负向证据全是不带本列的裸 INSERT——去掉默认值它们会先撞 NOT NULL，于是那些
-- CHECK 有没有在守就再也证不出来了。适配器自己每次都显式给值，不靠这个默认。

ALTER TABLE visibility_exception.claim_item
    ADD COLUMN revision bigint NOT NULL DEFAULT 1;

ALTER TABLE visibility_exception.claim_item
    ADD CONSTRAINT claim_item_revision_positive CHECK (revision >= 1);
