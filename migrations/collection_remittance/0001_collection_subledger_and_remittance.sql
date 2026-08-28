-- 代收与清分上下文的首张迁移（票 admin-remainder-mechanism-batch/04）。所有权取
-- CONTEXT 所有权句：本上下文拥有代收指令、代收事实的接受判断、代收分户账及其全部
-- 记账、回汇批次、汇付主张与差异事项；settlement-accounting 拥有 COD 服务费、抵扣、
-- 运营应收应付与经营核算。两边不共享表，本模块因此独立成 schema，不碰 SA 的任何表。
--
-- 五张表对应 CONTEXT 的五个词形：代收指令、代收事实、代收分户账（开立面）、分户账
-- 记账（余额面）、回汇批次与差异事项。**余额不落字段**——它是 subledger_posting 的
-- 派生结果；账上留一列可改写的余额，就等于给绕过记账的写入路径开了门，而分配守恒
-- 恰恰只在「金额只经记账进出」时才守得住。
--
-- 真实客户、责任法人、代收渠道、币种与金额全部属实例半边（PA-CR-01 形态已准入，
-- 实例值一律留空）；本迁移不含任何实例默认值，隔离演示走 SYN- 前缀合成种子
-- （ADR-0078）。汇率、回汇周期、手续费与支付通道不进本模块——它们不是本上下文的
-- 事实，任何一列都会变成替租户拟的商业口径。

-- 代收指令：一行即「依据某份客户代收服务要求，该包裹应向收件人收取此金额」。
-- requirement_ref 指向 parcel-shipment 拥有的客户代收服务要求快照，parcel_ref 指向
-- 包裹身份——两者都是标识引用，跨上下文不设外键（所有权在别处，本模块只引用不校验
-- 在册性）。指令是义务登记，不是收款事实：实收由 collection_fact 表达。
CREATE TABLE collection_remittance.collection_instruction (
    tenant_id        text        NOT NULL,
    instruction_ref  text        NOT NULL,

    parcel_ref       text        NOT NULL,
    requirement_ref  text        NOT NULL,
    -- 分户账四维随指令固定：本金记进哪一本账在义务成立时就已确定，不由后续记账挑选。
    customer_ref     text        NOT NULL,
    legal_entity_ref text        NOT NULL,
    currency         text        NOT NULL,
    channel_ref      text        NOT NULL,
    amount_minor     bigint      NOT NULL,
    instructed_at    timestamptz NOT NULL,

    CONSTRAINT collection_instruction_pkey
        PRIMARY KEY (tenant_id, instruction_ref),

    CONSTRAINT collection_instruction_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(instruction_ref) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(requirement_ref) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(channel_ref) <> ''
        ),

    CONSTRAINT collection_instruction_amount_positive
        CHECK (amount_minor > 0)
);

-- 代收事实：一行即「某一层来源报来了这笔实收」。source_layer 是**封闭四值**，词表
-- 来自 GLOSSARY「代收货款」那条硬句——收件人付款、渠道报告已代收、渠道通知已回款、
-- 运营企业真实到账是四层不同事实，不得互相覆盖或直接推导。它不是租户实例词表，因此
-- 写进 CHECK 不算替租户拟默认值；反过来，缺了这一列就没有任何东西能拦住「按 POD 推
-- 定已收款」那一类。
--
-- 挂在指令上的外键是有意的：没有代收指令就没有代收义务，无来源的本金不许进库。孤儿
-- 事实撞上这道防线时写口答未决（先例：关务义务明细撞目录外键）。
CREATE TABLE collection_remittance.collection_fact (
    tenant_id       text        NOT NULL,
    fact_ref        text        NOT NULL,

    instruction_ref text        NOT NULL,
    source_layer    text        NOT NULL,
    evidence_ref    text        NOT NULL,
    currency        text        NOT NULL,
    amount_minor    bigint      NOT NULL,
    occurred_at     timestamptz NOT NULL,

    CONSTRAINT collection_fact_pkey
        PRIMARY KEY (tenant_id, fact_ref),

    CONSTRAINT collection_fact_instruction_fkey
        FOREIGN KEY (tenant_id, instruction_ref)
        REFERENCES collection_remittance.collection_instruction (tenant_id, instruction_ref),

    CONSTRAINT collection_fact_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(instruction_ref) <> ''
            AND btrim(evidence_ref) <> ''
            AND btrim(currency) <> ''
        ),

    CONSTRAINT collection_fact_source_layer_closed
        CHECK (source_layer IN (
            'RECIPIENT_PAYMENT',
            'CHANNEL_COLLECTION_REPORT',
            'CHANNEL_REMITTANCE_NOTICE',
            'OPERATOR_BANK_CREDIT'
        )),

    CONSTRAINT collection_fact_amount_positive
        CHECK (amount_minor > 0)
);

-- 代收分户账（开立面）：一行即「这四维上有一本受托保管账」。键就是四维本身——
-- 客户、责任法人、币种、代收渠道少任何一维，两笔本不该混的钱都会落进同一个余额，
-- 而事后按更细的维再拆分要重演全部记账历史（粒度取舍记 ADR-0082）。
--
-- 开立面与记账面分两张表，理由同关务义务的目录/明细：「已开立但当期无记账」与
-- 「未开立」是两个相反的答案，登记方必须能单独表达前者，否则读面只能用「查不到」
-- 冒充「无本金」。custody_basis_ref 指向 party-commercial 的代收商业责任依据。
CREATE TABLE collection_remittance.subledger (
    tenant_id         text        NOT NULL,
    customer_ref      text        NOT NULL,
    legal_entity_ref  text        NOT NULL,
    currency          text        NOT NULL,
    channel_ref       text        NOT NULL,

    custody_basis_ref text        NOT NULL,
    opened_at         timestamptz NOT NULL,

    CONSTRAINT subledger_pkey
        PRIMARY KEY (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref),

    CONSTRAINT subledger_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(customer_ref) <> ''
            AND btrim(legal_entity_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(channel_ref) <> ''
            AND btrim(custody_basis_ref) <> ''
        )
);

-- 回汇批次：一次周期归集与汇付主张。分户账键与币种随批次冻结，归集截点亦然——
-- 批次一经形成不接受删除、重开或改写，改变归集范围走新批次（不可覆盖形状记
-- ADR-0082）。state 是**单向状态推进**而不是覆盖：写口只允许 COLLECTED →
-- HANDED_FOR_PAYMENT，没有回退路径，键、币种、截点与形成时刻没有任何改写入口。
--
-- 成员不另立第二张表：批次成员就是「引用该批次的汇付记账」。两份成员口径必然会
-- 在某一次部分失败后彼此不一致，而对得上与对不上在读面上长着同一张脸。
--
-- 表里没有周期、手续费与汇付通道列：那三样属实例半边，落成列就得给默认值。
CREATE TABLE collection_remittance.remittance_batch (
    tenant_id         text        NOT NULL,
    batch_ref         text        NOT NULL,

    customer_ref      text        NOT NULL,
    legal_entity_ref  text        NOT NULL,
    currency          text        NOT NULL,
    channel_ref       text        NOT NULL,

    collected_through timestamptz NOT NULL,
    state             text        NOT NULL,
    formed_at         timestamptz NOT NULL,

    CONSTRAINT remittance_batch_pkey
        PRIMARY KEY (tenant_id, batch_ref),

    CONSTRAINT remittance_batch_subledger_fkey
        FOREIGN KEY (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref)
        REFERENCES collection_remittance.subledger
            (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref),

    CONSTRAINT remittance_batch_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(batch_ref) <> ''),

    CONSTRAINT remittance_batch_state_closed
        CHECK (state IN ('COLLECTED', 'HANDED_FOR_PAYMENT'))
);

-- 差异事项：实收≠指令时登记的待处置事项。它**不自动冲销**——短款不得靠缩小指令
-- 抹平，溢款不得直接计入应付客户款；差额落账要另有一笔以本事项为依据的记账。
CREATE TABLE collection_remittance.discrepancy_item (
    tenant_id       text        NOT NULL,
    discrepancy_ref text        NOT NULL,

    instruction_ref text        NOT NULL,
    kind            text        NOT NULL,
    currency        text        NOT NULL,
    amount_minor    bigint      NOT NULL,
    basis_ref       text        NOT NULL,
    observed_at     timestamptz NOT NULL,

    CONSTRAINT discrepancy_item_pkey
        PRIMARY KEY (tenant_id, discrepancy_ref),

    CONSTRAINT discrepancy_item_instruction_fkey
        FOREIGN KEY (tenant_id, instruction_ref)
        REFERENCES collection_remittance.collection_instruction (tenant_id, instruction_ref),

    CONSTRAINT discrepancy_item_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(discrepancy_ref) <> ''
            AND btrim(instruction_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(basis_ref) <> ''
        ),

    CONSTRAINT discrepancy_item_kind_closed
        CHECK (kind IN ('SHORTFALL', 'SURPLUS')),

    CONSTRAINT discrepancy_item_amount_positive
        CHECK (amount_minor > 0)
);

-- 分户账记账：一行即「一笔正金额自某个资金位置移到另一个资金位置」。整张表**只追加**
-- ——写口没有 UPDATE 也没有 DELETE 路径，写错走反向记账冲正，原记账原样留在账上。
-- 各位置余额由本表派生，分配守恒（非外部位置余额之和恒等于入账之和）因此由记账的
-- 形状交付，而不是靠事后对平一个可改写的余额列。
--
-- 资金位置封闭六值加一个外部来源。EXTERNAL_SOURCE 只出现在来源侧：本金只能凭一笔
-- 已接受的代收事实进账，账上不许出现无来源的金额；也没有任何记账把钱移出账外——
-- 已汇付是账内的终局位置，于是全账总额恒等于入账之和，守恒可在一条 SQL 上核完。
--
-- 四条依据门写进 CHECK 而不是只写进注释：入账必须凭代收事实、进入 REMITTED 必须凭
-- 回汇批次、进入 SHORTFALL/SURPLUS 必须凭差异事项，而 PAYABLE_TO_CUSTOMER **不得**
-- 凭代收事实到达——那一条正是「未实际收到的代收款不得进入应付客户」的结构落点：
-- 归属要另有一笔以代收指令为依据的清分记账，一层来源事实推不出可付客户余额。四件
-- 都是 CONTEXT 的硬句，能做进结构就不留给调用方自觉——留给自觉的那半在库里看不出
-- 守没守。
--
-- basis_ref 不设外键：basis_kind 五种依据分别指向代收事实、代收指令、回汇批次、
-- 差异事项与另一笔记账，一列上立不出五个方向的外键。按依据种类分派核对由写口完成
-- （写口读回依据行再落账），这一格的裁量与关务申报路径「以标识引用口岸、不设外键」
-- 同款。
CREATE TABLE collection_remittance.subledger_posting (
    tenant_id        text        NOT NULL,
    posting_ref      text        NOT NULL,

    customer_ref     text        NOT NULL,
    legal_entity_ref text        NOT NULL,
    currency         text        NOT NULL,
    channel_ref      text        NOT NULL,

    from_position    text        NOT NULL,
    to_position      text        NOT NULL,
    amount_minor     bigint      NOT NULL,
    basis_kind       text        NOT NULL,
    basis_ref        text        NOT NULL,
    posted_at        timestamptz NOT NULL,

    CONSTRAINT subledger_posting_pkey
        PRIMARY KEY (tenant_id, posting_ref),

    CONSTRAINT subledger_posting_subledger_fkey
        FOREIGN KEY (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref)
        REFERENCES collection_remittance.subledger
            (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref),

    CONSTRAINT subledger_posting_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(posting_ref) <> ''
            AND btrim(basis_ref) <> ''
        ),

    CONSTRAINT subledger_posting_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT subledger_posting_from_position_closed
        CHECK (from_position IN (
            'EXTERNAL_SOURCE',
            'IN_TRANSIT_AT_CHANNEL',
            'AWAITING_ALLOCATION',
            'PAYABLE_TO_CUSTOMER',
            'REMITTED',
            'SHORTFALL',
            'SURPLUS'
        )),

    -- 去向侧不含 EXTERNAL_SOURCE：没有任何记账把本金移出账外。
    CONSTRAINT subledger_posting_to_position_closed
        CHECK (to_position IN (
            'IN_TRANSIT_AT_CHANNEL',
            'AWAITING_ALLOCATION',
            'PAYABLE_TO_CUSTOMER',
            'REMITTED',
            'SHORTFALL',
            'SURPLUS'
        )),

    CONSTRAINT subledger_posting_positions_differ
        CHECK (from_position <> to_position),

    CONSTRAINT subledger_posting_basis_kind_closed
        CHECK (basis_kind IN (
            'COLLECTION_FACT',
            'ALLOCATION',
            'REMITTANCE_BATCH',
            'DISCREPANCY',
            'CORRECTION'
        )),

    CONSTRAINT subledger_posting_intake_needs_collection_fact
        CHECK (from_position <> 'EXTERNAL_SOURCE' OR basis_kind = 'COLLECTION_FACT'),

    -- 清分依据是代收指令（这笔钱按该指令归属该客户）。这道 CHECK 只挡住「凭一层来源
    -- 事实直接记成可付客户余额」这一种走法，不规定归属该由谁判——那是编排的事。
    CONSTRAINT subledger_posting_payable_needs_allocation
        CHECK (to_position <> 'PAYABLE_TO_CUSTOMER' OR basis_kind <> 'COLLECTION_FACT'),

    CONSTRAINT subledger_posting_remittance_needs_batch
        CHECK (to_position <> 'REMITTED' OR basis_kind = 'REMITTANCE_BATCH'),

    CONSTRAINT subledger_posting_variance_needs_discrepancy
        CHECK (to_position NOT IN ('SHORTFALL', 'SURPLUS') OR basis_kind = 'DISCREPANCY')
);

-- 余额派生按分户账键扫全部记账，两侧各一次；按键取记账是本表唯一的读法。
CREATE INDEX subledger_posting_by_subledger
    ON collection_remittance.subledger_posting
        (tenant_id, customer_ref, legal_entity_ref, currency, channel_ref);
