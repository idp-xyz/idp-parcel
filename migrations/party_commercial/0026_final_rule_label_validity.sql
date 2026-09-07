-- 终局规则声明长一格面单有效期（票 party-commercial-context-gaps/09，ADR-0119）：自某一起算时刻种类
-- 起经过一段时长，该包裹的成功面单结果不可逆失效——CONTEXT「接受时固定规则下的不可逆失效」所依据的
-- 那条规则，此前语言在、格没有。
--
-- 落在 final_rule_content **父行**而不是子表：一版终局规则至多一条有效期，它说的是「这份规则下成功面单
-- 多久失效」，不按责任结果分——子表那一维答「哪种结果形成终局」，两维正交，硬拼进子表会让同一个时长在
-- 四行里重复四遍且必须相等。不另开 1:1 子表：缺席已由两列同为 NULL 说清，多一张表只多一种表达法。
-- 不改已施加的 0013：ALTER TABLE 加列，既有父行两列皆 NULL，读回即「未声明有效期」，语义一字不变。
--
-- 两列同在同缺（形照 0022 汇率口径三格全有或全无）：只有种类没有时长、或只有时长没有种类，都不是一条
-- 算得出东西的声明，进不来。时长严格为正：零与负数不是「多久失效」的答案（domain.NewLabelValidityDeclaration
-- 同判据）。种类 CHECK 镜像 domain.ValidityAnchorKind 首发一值——起算时刻首发只开「渠道结果业务时间」
--（MCP-1 代裁 Q1），加格是新一版声明的事，放宽只改 CHECK，既有行一字不动。
--
-- 时长用 interval 而不是整数天：锚是带时刻精度的业务时间，「N 天」是租户的选择不是产品的假设。批文口只收
-- ISO-8601 子集（禁年 / 月 / 周），所以本列不会出现月份段；读回若见月份段即库与写口分叉，装载报错不吸收。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼：两个可空列先 IS NULL / IS NOT NULL 再比较，没有一条比较式能单独以
-- NULL 决定约束。
--
-- 行属实例半边：要不要声明、多久失效由租户按 PAR-COM-17 登记，今天没有租户因而这两列全空；表上没有
-- 任何默认时长——缺席就是不失效，判失效归 parcel-shipment 读时按声明算。

ALTER TABLE party_commercial.final_rule_content
    ADD COLUMN validity_anchor   text,
    ADD COLUMN validity_duration interval;

ALTER TABLE party_commercial.final_rule_content
    ADD CONSTRAINT final_rule_content_validity_all_or_none
        CHECK (
            (validity_anchor IS NULL AND validity_duration IS NULL)
            OR (validity_anchor IS NOT NULL AND validity_duration IS NOT NULL)
        );

-- 镜像 domain.ValidityAnchorKind，首发一值。
ALTER TABLE party_commercial.final_rule_content
    ADD CONSTRAINT final_rule_content_validity_anchor_closed
        CHECK (validity_anchor IS NULL OR validity_anchor IN ('CHANNEL_RESULT_OBSERVED'));

ALTER TABLE party_commercial.final_rule_content
    ADD CONSTRAINT final_rule_content_validity_duration_positive
        CHECK (validity_duration IS NULL OR validity_duration > interval '0');
