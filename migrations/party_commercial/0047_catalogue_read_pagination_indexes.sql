-- 票 catalogue-read-pagination-rollout/11：party-commercial 其余十八册迁到 ADR-0144 之后，各册按「租户 + 排序维 + 行标识」
-- 取页，每迁一册补一条与缺省序同形的索引（ADR-0144 决定七）。已有同形前缀而缺标识段的，换成补齐标识段的同名索引，
-- 不让两条前缀相同的索引在写路径上各付一次。只建索引，不动数据。
--
-- 参与方身份三册、产品渠道映射与注册号类型五册的读面先按主键把每个对象折叠到最新修订再排，外层的排序穿不过折叠，
-- 取页与计数都用不上这五条：它们按决定七补齐了形状，还没有读者，与 0035 客户账户那条同一处境；按修订折叠的册该配
-- 什么索引，与 ADR-0144 越权风险点 3 同一前提，量级上来时一并定。

-- 业务参与方：缺省序登记时刻倒序，标识参与方号。
CREATE INDEX business_party_registration_by_recorded_at
    ON party_commercial.business_party_registration (tenant_id, recorded_at DESC, party_id DESC);

-- 集团法人：缺省序登记时刻倒序，标识法人号。
CREATE INDEX legal_entity_registration_by_recorded_at
    ON party_commercial.legal_entity_registration (tenant_id, recorded_at DESC, legal_entity_id DESC);

-- 参与方关系：缺省序登记时刻倒序，标识关系号。
DROP INDEX party_commercial.party_relationship_registration_by_tenant;
CREATE INDEX party_relationship_registration_by_tenant
    ON party_commercial.party_relationship_registration (tenant_id, recorded_at DESC, relationship_id DESC);

-- 产品渠道映射：缺省序登记时刻倒序，标识映射号。
DROP INDEX party_commercial.product_channel_mapping_registration_by_tenant;
CREATE INDEX product_channel_mapping_registration_by_tenant
    ON party_commercial.product_channel_mapping_registration (tenant_id, recorded_at DESC, mapping_id DESC);

-- 注册号类型：缺省序登记时刻倒序，标识（国家、类型码）。
DROP INDEX party_commercial.registration_number_type_registration_by_tenant;
CREATE INDEX registration_number_type_registration_by_tenant
    ON party_commercial.registration_number_type_registration
        (tenant_id, recorded_at DESC, country_code DESC, type_code DESC);

-- 版本壳六册（服务产品、客户合同、供应商协议、授权规则、客户服务规则、接受前财务控制策略）由同一张版本表按类别分册：
-- 类别是等值前缀，缺省序发布时刻倒序，标识（对象、版本）。
CREATE INDEX commercial_version_by_kind_published
    ON party_commercial.commercial_version
        (tenant_id, object_kind, published_at DESC, object_id DESC, version_label DESC);

-- 接单规则包：缺省序声明时刻倒序，标识（对象、版本）。
CREATE INDEX acceptance_rule_package_by_declared
    ON party_commercial.acceptance_rule_package (tenant_id, declared_at DESC, object_id DESC, version_label DESC);

-- 接受前财务控制声明：缺省序声明时刻倒序，标识（合同对象、合同版本）。
CREATE INDEX pre_acceptance_control_declaration_by_declared
    ON party_commercial.pre_acceptance_control_declaration
        (tenant_id, declared_at DESC, object_id DESC, version_label DESC);

-- 价格政策：缺省序登记时刻倒序，标识（对象、版本）。
CREATE INDEX commercial_price_policy_by_registered
    ON party_commercial.commercial_price_policy (tenant_id, registered_at DESC, object_id DESC, version_label DESC);

-- 结算政策：缺省序登记时刻倒序，标识（对象、版本）。
CREATE INDEX commercial_settlement_policy_by_registered
    ON party_commercial.commercial_settlement_policy
        (tenant_id, registered_at DESC, object_id DESC, version_label DESC);

-- 时点锚声明：缺省序声明时刻倒序，标识（规则包对象、规则包版本、判断类型）。
CREATE INDEX as_of_policy_declaration_by_declared
    ON party_commercial.as_of_policy_declaration
        (tenant_id, declared_at DESC, object_id DESC, version_label DESC, judgment_type DESC);

-- 信用政策：缺省序登记时刻倒序，标识（对象、版本）。
CREATE INDEX credit_policy_by_registered
    ON party_commercial.credit_policy (tenant_id, registered_at DESC, object_id DESC, version_label DESC);

-- 开放中的发布载体：只列待批准与已批准两格。部分索引的谓词与读面写进语句的条件同为字面量（读面从领域枚举取码），
-- 规划器才认得出这条索引可用；已发布的载体越积越多，不进这条索引。
CREATE INDEX publication_draft_open_by_submitted
    ON party_commercial.publication_draft
        (tenant_id, submitted_at DESC, object_kind DESC, object_id DESC, version_label DESC)
    WHERE status IN (1, 2);
