-- 客户合同版本的销售价格约定（ADR-0380 决定一，票 commercial-four-module-requirements/16）。
--
-- 一份合同版本对「包裹的 SELL 客户费用按哪几个计价范围、依什么顺序取价」的声明，二选一：一份有序的销售价格来源，
-- 或显式不计销售价并带依据。它是合同版本正文的一层，与 0007（合同级「要不要」接受前控制）、0012（正文）、0030
-- （合同层交付条件）并列不并表——各层各自可缺。无父行 = 这一版合同没有销售价格约定（加这一层之前发布的合同都是
-- 这一种），读面如实答「没有」，不补默认。
--
-- 形照 0012 / 0030：父行是约定的壳，子行是来源，一行一个计价范围。两格恰一由父行与子表合起来表达：
-- no_sell_price_basis 非空 = 显式不计销售价，此时没有子行；为空 = 有来源，此时至少一条子行。这是跨表条件，CHECK
-- 表达不了，由写口先过领域构造门（DeclareContractSellPrice）再写；读到父子对不上的行即坏数据，读面上抛，不折成
-- 「没有」。
--
-- ordinal 是来源在合同声明里的位置，从 1 起。顺序是正文——回退只在声明的顺序里发生（ADR-0380 决定三）——所以它
-- 进主键，读面按它排，不按范围名排。同一计价范围不出现两次，由唯一约束结构性挡住。
--
-- pricing_scope_ref 是开放引用：指名计价一侧价卡登记册选卡键上的计价范围，本上下文不核它在那一侧有没有卡
-- （ADR-0380 决定一）。它不是 commercial_version.scope_ref 那种商业范围。
--
-- 随版本发布登记，更正走新版本，不开行级改写。行属实例半边：表上没有任何默认范围。
--
-- 号段跳过 0048：取号时它已被一条未进 main 的分支占用。

CREATE TABLE party_commercial.customer_contract_sell_price_arrangement (
    tenant_id            text        NOT NULL,
    object_kind          smallint    NOT NULL,
    object_id            text        NOT NULL,
    version_label        text        NOT NULL,

    no_sell_price_basis  text,
    declared_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customer_contract_sell_price_arrangement_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label),

    -- 可空列先 IS NULL 再用：依据在场时不得为空白。
    CONSTRAINT customer_contract_sell_price_arrangement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(object_id) <> ''
            AND btrim(version_label) <> ''
            AND (no_sell_price_basis IS NULL OR btrim(no_sell_price_basis) <> '')
        ),

    -- 2 = CustomerContractObject。
    CONSTRAINT customer_contract_sell_price_arrangement_contract_only
        CHECK (object_kind = 2)
);

-- 一行是一个销售价格来源：合同在第几位指名了哪个计价范围。
CREATE TABLE party_commercial.customer_contract_sell_price_source (
    tenant_id          text     NOT NULL,
    object_kind        smallint NOT NULL,
    object_id          text     NOT NULL,
    version_label      text     NOT NULL,
    ordinal            integer  NOT NULL,

    pricing_scope_ref  text     NOT NULL,

    CONSTRAINT customer_contract_sell_price_source_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, ordinal),

    CONSTRAINT customer_contract_sell_price_source_scope_once
        UNIQUE (tenant_id, object_kind, object_id, version_label, pricing_scope_ref),

    CONSTRAINT customer_contract_sell_price_source_arrangement_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.customer_contract_sell_price_arrangement
            (tenant_id, object_kind, object_id, version_label)
        ON DELETE CASCADE,

    CONSTRAINT customer_contract_sell_price_source_ordinal_positive
        CHECK (ordinal >= 1),

    CONSTRAINT customer_contract_sell_price_source_not_blank
        CHECK (btrim(pricing_scope_ref) <> '')
);
