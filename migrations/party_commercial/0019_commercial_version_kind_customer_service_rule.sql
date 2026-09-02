-- 把发布登记册的对象类别约束对齐到扩大后的领域封闭集（票 party-commercial-context-gaps/04、
-- ADR-0093 的归族判据）。
--
-- 0004 把上界从 7 放到 9 时写下的那条理由在这里逐字重演：CommercialObjectKind 增至十类，
-- 第十类 CUSTOMER_SERVICE_RULE 是当前约束会拒的。这不是「库比领域严」，是**约束落后于它
-- 所镜像的那个集合**——CHECK 的职责是挡住封闭集之外的取值，而这个值就在集内。
--
-- 不改 0001，也不改 0004：已随提交落库的迁移正文按校验和守着，改写会让下一次运行以校验和
-- 不一致暴露（见 migrations.Asset）。放宽只能是一份新的不可变迁移，形照 0004。
--
-- 为什么客户服务规则版本该进这个集合而渠道账号使用授权不该：判据是**生命周期是不是版本
-- 演进**，不是持不持有 CommercialVersion（后者是表现，加个字段就能绕过去）。前者走草稿 →
-- 已发布 → 已生效 → 已到期或已替代，与商业版本共同生命周期逐格同形，且旧版本留在册上被
-- 既有案件、通知与索赔历史继续引用；后者走建立 → 撤销或自然到期，换了范围是另一笔授权而
-- 不是第二版，因此另立登记册（0018）。两者对照见 ADR-0093。
--
-- 本迁移只对齐约束，不接线：今天没有任何写入方往登记册放这一类，正文表与领域类型属本票
-- 后续切片。因此这一笔拆掉的是一个「等谁第一次发布这一类才会踩到」的陷阱，不是打开一条
-- 新通路。visibility-exception 那条断链（响应目标、披露决定、索赔期限本该以它为依据）
-- 另开一票，理由是把它并进来会把封闭集拓宽的变红窗口拉长到没必要的长度。

ALTER TABLE party_commercial.commercial_version
    DROP CONSTRAINT commercial_version_kind_known;

ALTER TABLE party_commercial.commercial_version
    ADD CONSTRAINT commercial_version_kind_known
        CHECK (object_kind BETWEEN 1 AND 10);
