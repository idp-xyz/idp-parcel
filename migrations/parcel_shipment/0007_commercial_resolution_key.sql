-- 消费方解析键登记面（syn-wall-door-audit 票 03 件 2）。ResolutionKeySource 是
-- parcel-shipment 自己的实例半边（ADR-0025：键与值属消费方），登记行因此落在本
-- 上下文的 schema，不落 party_commercial。
--
-- 一行把一个（租户+货主客户账户）映射到形成 party-commercial 闭包解析键所需的四项
-- 实例参数：商业范围、责任法人候选、选择锚点（策略版本+显式锚点时刻）与必需依据种类。
-- 无行 = 显式未配置：FormResolutionKey 交回 formed=false，解析停在
-- `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED`——首发要停下的地方，不是要绕过的地方。
--
-- anchor_at 是**显式登记的锚点时刻**，不是任何默认：UC-PC-002 明禁临时选用来源发生
-- 时间、客户请求时间或系统当前时间当默认锚点（AT-PC-018/019）。按请求变化的锚点语义
-- （如按提交时点）要求查询本身携带那个时点，那是给 CommercialBasisQuery 加维的另一个
-- 决定，不归本表；本表承载固定锚点的租户策略，语义由 anchor_policy_version 指名的
-- PAR-COM-14 政策版本负责，本上下文只携带引用不解释。
--
-- required_bases 镜像 domain.CommercialObjectKind 的封闭名集，但**刻意排除**
-- SETTLEMENT_POLICY 与 PRICE_RULE：这两类必需依据要求解析键额外携带结算选择器/价格
-- 方向（ADR-0044/0034「含则必填」），本登记面没有那些维度，放行它们等于登记一个
-- 永远立不起来的键。日后接通结算作用域缝时随新决定扩列，不在这里预留。
-- 空数组同样挡住：零必需依据的键连最小身份都立不起来。

CREATE TABLE parcel_shipment.commercial_resolution_key_registration (
    tenant_id             text        NOT NULL,
    customer_account_id   text        NOT NULL,

    scope_ref             text        NOT NULL,
    legal_entity_ref      text        NOT NULL,
    anchor_policy_version text        NOT NULL,
    anchor_at             timestamptz NOT NULL,
    required_bases        text[]      NOT NULL,
    registered_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commercial_resolution_key_registration_pkey
        PRIMARY KEY (tenant_id, customer_account_id),

    CONSTRAINT commercial_resolution_key_registration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_account_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(anchor_policy_version) <> ''
        ),

    CONSTRAINT commercial_resolution_key_registration_bases_not_empty
        CHECK (cardinality(required_bases) > 0),

    CONSTRAINT commercial_resolution_key_registration_bases_closed
        CHECK (required_bases <@ ARRAY[
            'SERVICE_PRODUCT',
            'CUSTOMER_CONTRACT',
            'SUPPLIER_AGREEMENT',
            'ACCEPTANCE_RULE_PACKAGE',
            'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
            'CREDIT_POLICY',
            'AUTHORIZATION_RULE'
        ]::text[])
);
