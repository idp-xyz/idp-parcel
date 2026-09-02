-- 把服务产品形态册的封闭集对齐到领域今日的两格（ADR-0088：面单渠道服务由 `PAR-COM-12`
-- 的范围裁剪改为纳入首发对客服务形态）。
--
-- 0008 写下 `form IN ('NETWORK_SERVICE')` 时领域只有一格，它的注释也写明「预先列上第二个
-- 值就是替租户拟一种它还没有的形态」——那句话在当时是对的。ADR-0088 之后第二格不再是
-- 「替租户拟」，它是已确认的范围决策，`domain.ServiceProductForm` 已随之扩到两格。
--
-- 不改 0008：已随提交落库的迁移正文按校验和守着，改写它会让 `migrate.verifyNoDrift` 在下一次
-- 运行时以 ErrChecksumDrift 阻断（先例与理由见 0004 放宽 0001 的 object_kind 时那一段）。
-- 放宽只能是一份新的不可变迁移。本迁移与 0008 因此是同一条约束的两份历史，读 0008 时要
-- 一并读本文件才知道封闭集今天有几格。
--
-- 只对齐约束，不接线：哪个产品是哪种形态仍属实例半边（`PAR-COM-05`/`PAR-COM-12` 的取值侧），
-- 本迁移不写入任何行，也不改变「只登版本不登形态是一格合法的缺席」这条装载纪律。

ALTER TABLE party_commercial.service_product_form
    DROP CONSTRAINT service_product_form_closed;

ALTER TABLE party_commercial.service_product_form
    ADD CONSTRAINT service_product_form_closed
        CHECK (form IN ('NETWORK_SERVICE', 'LABEL_CHANNEL_SERVICE'));
