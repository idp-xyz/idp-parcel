-- 审核应付与供应商费用贷项：UC-SA-004 步 5–6 的两张判断库（接收与匹配在 0002 的
-- supplier_bill_reception）。
--
-- audited_payable 主键取（租户+应付身份）：同一应付身份只形成一次，撞键即`已有记录`
-- （ON CONFLICT 代数，ADR-0031）。另有唯一约束（租户+主张+行）：一行主张只成立一份应付
-- ——「审核通过部分账单：只对通过金额形成审核应付，其他范围保持原状态」反过来说就是通过
-- 的那一格不再第二次通过（AT-SA-089）；撞它同样是`已有记录`，编排按行读回先到者。
-- 列上没有付款、收款或净额字段：审核应付不表示供应商已付款，贷项也不净入它（CONTEXT
-- 「供应商费用贷项与原审核应付是两个独立且关联的金额结果」）。行不带主张版本：应付回指的
-- 是主张身份与行，版本冲突在接收那一层已经拒过。
--
-- supplier_credit_note 主键取（租户+贷项身份+版本）：贷项只追加，自己的更正是新版本；
-- 每行回指原应付与原主张（NOT NULL），结算账户与币种随原应付——贷项与原应付在结构上不可能
-- 指向两本账或两种币（AT-SA-090）。这里没有指向 audited_payable 的外键：两表都是本上下文的
-- 判断库，回指关系由编排在形成时核对、由读面按引用取回，外键会把「原应付已成立」这一步
-- 从编排的显式判断变成库的隐式拒收，而拒收那一格该有自己的结果名（提交矛盾）。
--
-- 金额恒正：贷项方向恒为贷（冲减应付），方向不进列——进了列就有了一个可以填「借」的位置，
-- 而 CONTEXT 把带方向的调整留给客户费用那一族。

CREATE TABLE settlement_accounting.audited_payable (
    tenant_id        text        NOT NULL,
    payable_id       text        NOT NULL,

    claim_id         text        NOT NULL,
    line_ref         text        NOT NULL,
    expected_version text        NOT NULL,
    legal_entity     text        NOT NULL,
    account_id       text        NOT NULL,
    currency         text        NOT NULL,
    amount_minor     bigint      NOT NULL,
    auditor_ref      text        NOT NULL,
    audited_at       timestamptz NOT NULL,

    content_digest   text        NOT NULL,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT audited_payable_pkey
        PRIMARY KEY (tenant_id, payable_id),

    -- 一行主张只成立一份应付。
    CONSTRAINT audited_payable_one_per_line
        UNIQUE (tenant_id, claim_id, line_ref),

    CONSTRAINT audited_payable_identity_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(payable_id) <> ''
            AND btrim(claim_id) <> ''
            AND btrim(line_ref) <> ''
            AND btrim(expected_version) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(auditor_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 审核只裁决要不要付，改不了数字；零额或负额的「应付」不是应付（FormAuditedPayable 的库面）。
    CONSTRAINT audited_payable_amount_positive
        CHECK (amount_minor > 0)
);

CREATE TABLE settlement_accounting.supplier_credit_note (
    tenant_id      text        NOT NULL,
    note_id        text        NOT NULL,
    note_version   text        NOT NULL,

    payable_id     text        NOT NULL,
    claim_id       text        NOT NULL,
    account_id     text        NOT NULL,
    currency       text        NOT NULL,
    amount_minor   bigint      NOT NULL,
    reason_ref     text        NOT NULL,
    issued_at      timestamptz NOT NULL,

    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,

    CONSTRAINT supplier_credit_note_pkey
        PRIMARY KEY (tenant_id, note_id, note_version),

    CONSTRAINT supplier_credit_note_identity_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(note_id) <> ''
            AND btrim(note_version) <> ''
            AND btrim(payable_id) <> ''
            AND btrim(claim_id) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(reason_ref) <> ''
            AND btrim(content_digest) <> ''
        ),

    -- 贷记金额恒正（FormSupplierCreditNote 的库面）。
    CONSTRAINT supplier_credit_note_amount_positive
        CHECK (amount_minor > 0)
);

-- 按原应付取全部贷项：UC-SA-005 核销与 UC-SA-006 已确认口径都要「与其关联的当前有效
-- 供应商费用贷项」这一读法。
CREATE INDEX supplier_credit_note_by_payable
    ON settlement_accounting.supplier_credit_note (tenant_id, payable_id);
