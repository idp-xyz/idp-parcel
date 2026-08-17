-- 把发布登记册的对象类别约束对齐到领域封闭集（CONTEXT：接单规则包、接受前财务控制
-- 策略、商业价格政策、定价方案绑定、结算政策、信用政策和授权规则都遵循商业版本共同
-- 不变量）。
--
-- 0001 写下 `object_kind BETWEEN 1 AND 7` 时领域只有七类；此后 CommercialObjectKind
-- 增至九类，第八类 CREDIT_POLICY 与第九类 AUTHORIZATION_RULE 是这条约束今天会拒的。
-- 这不是「库比领域严」，是**约束落后于它所镜像的那个集合**：CHECK 的职责是挡住封闭集
-- 之外的取值，而这两个值就在集内。
--
-- 不改 0001：已随提交落库的迁移正文按校验和守着，改写它会让下一次运行以校验和不一致
-- 暴露（见 migrations.Asset）。放宽只能是一份新的不可变迁移。
--
-- 本迁移只对齐约束，不接线：今天没有任何写入方往登记册放这两类——授权规则的生效授予
-- 走 0003 的 authorization_grant（那张表自带足以重建 AuthorityGrant 的快照），信用政策
-- 尚无存储口。因此这一笔拆掉的是一个「等谁第一次发布这两类才会踩到」的陷阱，而不是打开
-- 一条新通路；把授权规则版本接进发布登记册仍属另票。

ALTER TABLE party_commercial.commercial_version
    DROP CONSTRAINT commercial_version_kind_known;

ALTER TABLE party_commercial.commercial_version
    ADD CONSTRAINT commercial_version_kind_known
        CHECK (object_kind BETWEEN 1 AND 9);
