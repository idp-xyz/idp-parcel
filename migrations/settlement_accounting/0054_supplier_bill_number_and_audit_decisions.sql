-- 供应商账单号册与审核决定册（ADR-0357；票 document-numbering/13）。
--
-- 供应商账单号是主张的外部标识：类型就是这张册，分配方是 supplier_ref（这份主张的供应商），被标识对象是 claim_id（主张身份，
-- 不是某一版），适用范围是这家供应商自己的账单号——主键（租户，供应商，账单号）即「同一供应商同号只标识一份主张」，不同供应商
-- 同号互不相干；（租户，主张）唯一即「一份主张只有一个号」。claim_version 记首次带来这个号的那一版，只作留痕。册由接收编排在
-- 接收事务里写，先于接收行；查重在编排里按号与主张排过队之后判，两道唯一是最后一道保险。本记录之前接收的主张不回填：票 11
-- 管理台把主张标识生成为「供应商/账单号」只是一时的借法，别的来路的主张标识不守这个形，拆出来的号没有分配方可言。
--
-- 审核决定：授权审核人对一行主张金额上的一段 [range_from_minor, range_to_minor) 给出通过、争议或拒绝。决定按（主张，行）归属、
-- 不分版本——审核应付本就如此回指（0016 头注）；claim_version 记审的时候点名的那一版。范围守恒由两道排他约束在库上兜底：同一行上
-- 直接给的各段（resolves_decision 为空）互不重叠；同一争议的各份再审互不重叠。再审落在原争议之内、通过不超已匹配金额，要读
-- 接收行与那份争议才判得了，归领域 DecideBillLineAudit。通过的决定，它的身份就是所形成审核应付的身份（audited_payable.payable_id）。
-- 操作证据判据同 0050：只作审计。
--
-- 已有审核应付回填为「通过这一行整段」的决定：0016 之下一行只成立一份应付、金额就是这一行的主张金额，所以 [0, amount_minor)
-- 就是它当时通过的那一段；标识、摘要、审核授权方、操作证据与时刻照搬，摘要照搬让旧载荷的重放照旧认得。当时按哪一版审的没有记，
-- claim_version 留空，读作不知道。
--
-- 一行可以有几份审核应付（一段一份），audited_payable_one_per_line 撤下，范围不重叠改由审核决定册担保；另建按行的普通索引
-- 给读面用。施加在有数据的库上：只建两张新表、回填新表、撤一道唯一约束、建索引，不改任何已有行的内容。
--
-- gist 上的文本等值列要 btree_gist；IF NOT EXISTS 理由同 node_operations 0013 那一处。

CREATE TABLE settlement_accounting.supplier_bill_number (
    tenant_id     text        NOT NULL,
    supplier_ref  text        NOT NULL,
    bill_number   text        NOT NULL,
    claim_id      text        NOT NULL,
    claim_version text        NOT NULL,
    recorded_at   timestamptz NOT NULL,

    CONSTRAINT supplier_bill_number_pkey
        PRIMARY KEY (tenant_id, supplier_ref, bill_number),

    CONSTRAINT supplier_bill_number_one_per_claim
        UNIQUE (tenant_id, claim_id),

    CONSTRAINT supplier_bill_number_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(supplier_ref) <> ''
            AND btrim(bill_number) <> ''
            AND btrim(claim_id) <> ''
            AND btrim(claim_version) <> ''
        ),

    -- 账单号逐字保存（领域 NewSupplierBillNumber 的库面）：首尾不带空白——Go unicode.IsSpace 认的那几种，\s 之外逐个列出——
    -- 中间不含控制字符（C0、C1；NUL 本就进不了 text）与格式字符（Unicode 类别 Cf，照 Unicode 15 的码位逐段列出）。查重逐字比，
    -- 看不见的字符收进来就能让同一个号再登一次。正则用 ARE 的 \u、\U 码位写法，标准字符串里反斜杠原样交给正则。
    CONSTRAINT supplier_bill_number_verbatim
        CHECK (
            bill_number !~ '^[\s\u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]|[\s\u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]$'
            AND bill_number !~ '[\u0001-\u001F\u007F-\u009F\u00AD\u0600-\u0605\u061C\u06DD\u070F\u0890-\u0891\u08E2\u180E\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u206F\uFEFF\uFFF9-\uFFFB\U000110BD\U000110CD\U00013430-\U0001343F\U0001BCA0-\U0001BCA3\U0001D173-\U0001D17A\U000E0001\U000E0020-\U000E007F]'
        )
);

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE settlement_accounting.supplier_bill_audit_decision (
    tenant_id          text        NOT NULL,
    decision_id        text        NOT NULL,

    claim_id           text        NOT NULL,
    claim_version      text,
    line_ref           text        NOT NULL,
    verdict            text        NOT NULL,
    range_from_minor   bigint      NOT NULL,
    range_to_minor     bigint      NOT NULL,
    basis_ref          text,
    resolves_decision  text,
    auditor_ref        text        NOT NULL,
    operation_evidence text,
    audited_at         timestamptz NOT NULL,

    content_digest     text        NOT NULL,
    recorded_at        timestamptz NOT NULL,

    CONSTRAINT supplier_bill_audit_decision_pkey
        PRIMARY KEY (tenant_id, decision_id),

    CONSTRAINT supplier_bill_audit_decision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(decision_id) <> ''
            AND btrim(claim_id) <> ''
            AND btrim(line_ref) <> ''
            AND btrim(auditor_ref) <> ''
            AND btrim(content_digest) <> ''
            AND (claim_version IS NULL OR btrim(claim_version) <> '')
            AND (basis_ref IS NULL OR btrim(basis_ref) <> '')
            AND (resolves_decision IS NULL OR btrim(resolves_decision) <> '')
            AND (operation_evidence IS NULL OR btrim(operation_evidence) <> '')
        ),

    -- 三值封闭集与领域 AuditVerdict 同笔改。
    CONSTRAINT supplier_bill_audit_decision_verdict_closed
        CHECK (verdict IN ('PASSED', 'DISPUTED', 'REJECTED')),

    CONSTRAINT supplier_bill_audit_decision_range_ordered
        CHECK (range_from_minor >= 0 AND range_to_minor > range_from_minor),

    -- 争议与拒绝必须带依据；再审只能通过或拒绝、必须带依据、不解自己（领域 BillLineAuditDecision 的库面）。
    CONSTRAINT supplier_bill_audit_decision_basis_present
        CHECK (verdict = 'PASSED' OR basis_ref IS NOT NULL),
    CONSTRAINT supplier_bill_audit_decision_resolution_shaped
        CHECK (resolves_decision IS NULL
               OR (verdict <> 'DISPUTED' AND basis_ref IS NOT NULL AND resolves_decision <> decision_id)),

    CONSTRAINT supplier_bill_audit_decision_line_ranges_disjoint
        EXCLUDE USING gist (
            tenant_id WITH =,
            claim_id WITH =,
            line_ref WITH =,
            int8range(range_from_minor, range_to_minor) WITH &&
        ) WHERE (resolves_decision IS NULL),

    CONSTRAINT supplier_bill_audit_decision_reaudits_disjoint
        EXCLUDE USING gist (
            tenant_id WITH =,
            resolves_decision WITH =,
            int8range(range_from_minor, range_to_minor) WITH &&
        ) WHERE (resolves_decision IS NOT NULL)
);

CREATE INDEX supplier_bill_audit_decision_by_line
    ON settlement_accounting.supplier_bill_audit_decision (tenant_id, claim_id, line_ref);

INSERT INTO settlement_accounting.supplier_bill_audit_decision
    (tenant_id, decision_id, claim_id, claim_version, line_ref, verdict, range_from_minor, range_to_minor,
     basis_ref, resolves_decision, auditor_ref, operation_evidence, audited_at, content_digest, recorded_at)
SELECT tenant_id, payable_id, claim_id, NULL, line_ref, 'PASSED', 0, amount_minor,
       NULL, NULL, auditor_ref, operation_evidence, audited_at, content_digest, recorded_at
  FROM settlement_accounting.audited_payable;

ALTER TABLE settlement_accounting.audited_payable
    DROP CONSTRAINT audited_payable_one_per_line;

CREATE INDEX audited_payable_by_line
    ON settlement_accounting.audited_payable (tenant_id, claim_id, line_ref);
