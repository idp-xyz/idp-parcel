-- 税费付款协作、外部资金事实引用与税费付款核对三张表（票 mechanism-executor-triage/07 CC-c，
-- UC-CC-009 步 4–7）。所有权取 UC-CC-009「七层对象必须分离」：付款协作、外部资金事实、关务
-- 付款判断是三层，各自追加保存，谁都不是「税费状态」——分三张表正是那节在库面上的样子。
-- 此前关务迁移无这三层的落点，领域工厂 FormDutyCollaboration / VerifyDutyPayment 只被测试
-- 调过。
--
-- 真实程序的付款条件、关联规则、币种与金额全部属实例半边，本迁移不含任何实例默认值；
-- 隔离演示走 SYN- 前缀合成种子（ADR-0078）。

-- 税费付款协作事项：一行即针对一个范围、依据一份税费义务依据形成的内部协作对象。义务依据
-- 两格（CONTEXT「税费付款协作事项」语言）：核定税费格带税费引用不带无需付款依据，明确无需
-- 付款格反之——第三种「没有结果所以不用付」在类型上没有格，库里也没有：CHECK 只放这两种形状。
-- 键上的 duty_ref 在无需付款格为空串，让两格在同一范围上各占一行；税费更正换税费引用即换行，
-- 历史不覆盖。它不等于支付指令、付款交易、客户回收或监管放行——表上没有那些列。
CREATE TABLE customs_compliance.duty_payment_collaboration (
    tenant_id       text        NOT NULL,
    scope_ref       text        NOT NULL,
    duty_ref        text        NOT NULL DEFAULT '',

    kind            text        NOT NULL,
    no_pay_basis    text        NOT NULL DEFAULT '',
    obligor_ref     text        NOT NULL,
    requirement_ref text        NOT NULL,
    target_ref      text        NOT NULL,
    formed_at       timestamptz NOT NULL,

    CONSTRAINT duty_payment_collaboration_pkey
        PRIMARY KEY (tenant_id, scope_ref, duty_ref),

    CONSTRAINT duty_payment_collaboration_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(obligor_ref) <> ''
            AND btrim(requirement_ref) <> ''
            AND btrim(target_ref) <> ''
        ),

    -- 义务依据封闭二值，与 domain.DutyObligationKind 的 String 同词。
    CONSTRAINT duty_payment_collaboration_kind_closed
        CHECK (kind IN ('ASSESSED_DUTY', 'EXPLICITLY_NOT_REQUIRED')),

    -- 两格各自的形状，与领域 FormDutyCollaboration 同判据。
    CONSTRAINT duty_payment_collaboration_basis_matches_kind
        CHECK (
            (kind = 'ASSESSED_DUTY' AND btrim(duty_ref) <> '' AND no_pay_basis = '')
            OR (kind = 'EXPLICITLY_NOT_REQUIRED' AND duty_ref = '' AND btrim(no_pay_basis) <> '')
        )
);

-- 外部资金事实引用：一行即银行/支付/财务系统一条事实到本上下文的入向登记（UC-CC-009 步 6 的
-- CC 半边）。真实付款归来源拥有，这里只登引用与关联核对最少要读的业务语义；没有状态列——
-- 「待关联」是派生的：没有任何核对行引用它的事实就是待关联。金额按币种最小单位计，允许为零
-- （付款失败、撤销这类来源事实本就没有正向金额），为负是矛盾输入。
CREATE TABLE customs_compliance.external_funds_fact (
    tenant_id    text        NOT NULL,
    fact_ref     text        NOT NULL,

    source_ref   text        NOT NULL,
    payer_ref    text        NOT NULL,
    currency     text        NOT NULL,
    amount_minor bigint      NOT NULL,
    occurred_at  timestamptz NOT NULL,
    received_at  timestamptz NOT NULL,

    CONSTRAINT external_funds_fact_pkey
        PRIMARY KEY (tenant_id, fact_ref),

    CONSTRAINT external_funds_fact_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(fact_ref) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(payer_ref) <> ''
            AND btrim(currency) <> ''
        ),

    CONSTRAINT external_funds_fact_amount_not_negative
        CHECK (amount_minor >= 0)
);

-- 税费付款核对：一行即一版核对——覆盖、差额、有效性三轴分立（CONTEXT 硬句 214 明禁互斥总
-- 状态，三列三个封闭集正是那半句的库面）。键是核对身份三维加内容指纹：同三维同内容重放不出
-- 第二版，迟到事实换轴即换指纹追加新版，不按到达顺序覆盖。basis 是「凭什么关联」的证据引用
-- ——金额相等不能单独作为关联，无依据的核对进不来（非空 CHECK）。外键钉「没有接收的资金
-- 事实就没有核对」；协作事项那道前置由编排守（税费更正换引用后旧核对仍要留着，不能外键到
-- 会换行的那张表上）。
CREATE TABLE customs_compliance.duty_payment_verification (
    tenant_id      text        NOT NULL,
    duty_ref       text        NOT NULL,
    funds_ref      text        NOT NULL,
    scope_ref      text        NOT NULL,
    version_digest text        NOT NULL,

    coverage       text        NOT NULL,
    delta          text        NOT NULL,
    validity       text        NOT NULL,
    basis          text        NOT NULL,
    verified_at    timestamptz NOT NULL,

    CONSTRAINT duty_payment_verification_pkey
        PRIMARY KEY (tenant_id, duty_ref, funds_ref, scope_ref, version_digest),

    CONSTRAINT duty_payment_verification_funds_fact_received
        FOREIGN KEY (tenant_id, funds_ref)
        REFERENCES customs_compliance.external_funds_fact (tenant_id, fact_ref),

    CONSTRAINT duty_payment_verification_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(duty_ref) <> ''
            AND btrim(funds_ref) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(version_digest) <> ''
            AND btrim(basis) <> ''
        ),

    -- 三轴各自封闭，与 domain.DutyCoverage / DutyDelta / DutyFactValidity 的 String 同词。
    CONSTRAINT duty_payment_verification_coverage_closed
        CHECK (coverage IN ('NONE', 'PARTIAL', 'COVERED')),
    CONSTRAINT duty_payment_verification_delta_closed
        CHECK (delta IN ('NO_DELTA', 'SHORT', 'EXCESS', 'PENDING')),
    CONSTRAINT duty_payment_verification_validity_closed
        CHECK (validity IN ('VALID', 'INVALIDATED', 'CONFLICTING', 'PENDING'))
);
