-- 关闭义务目录与放行门禁前置条件目录。两处共用同一个形状：**目录表 + 明细表**。
--
-- 分两张表是为了把「目录未登记」与「目录登记了但本次盘出空清单」分开。压成一张表就
-- 只剩「有没有明细行」一个信号，而这两格的含义相反：
--   * 义务目录未登记 → 未决，绝不是「没有义务所以可关」（CONTEXT 硬句 218 要求逐项
--     盘点全部适用义务；盘不出清单就不具备形成关闭决定的前提）。
--   * 门禁目录未登记 → 未决，没有清单的门禁判断无从复核；而登记了却空清单是
--     「此动作在此边界本就不受门禁」的如实答案（领域折叠为`不适用`）。
-- 目录行的在场因此是独立于明细的一条信息，必须自己占一行。

CREATE TABLE customs_compliance.closure_obligation_catalog (
    tenant_id    text        NOT NULL,
    case_ref     text        NOT NULL,

    registered_at timestamptz NOT NULL,

    CONSTRAINT closure_obligation_catalog_pkey
        PRIMARY KEY (tenant_id, case_ref),

    CONSTRAINT closure_obligation_catalog_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(case_ref) <> '')
);

CREATE TABLE customs_compliance.closure_obligation_item (
    tenant_id     text        NOT NULL,
    case_ref      text        NOT NULL,
    obligation    text        NOT NULL,

    scope         text        NOT NULL,
    item_state    text        NOT NULL,
    basis         text        NOT NULL,
    handed_to     text,
    -- 盘点按业务截点进行：适用区间决定某项义务在该截点是否进入本次清单。没有截点的
    -- 盘点说不清「截至什么时候」（领域 VerifyClosure 因此强制 cutoffAt）。
    applies_from  timestamptz NOT NULL,
    applies_until timestamptz,

    CONSTRAINT closure_obligation_item_pkey
        PRIMARY KEY (tenant_id, case_ref, obligation),
    CONSTRAINT closure_obligation_item_catalog_exists
        FOREIGN KEY (tenant_id, case_ref)
        REFERENCES customs_compliance.closure_obligation_catalog (tenant_id, case_ref),

    CONSTRAINT closure_obligation_item_not_blank
        CHECK (
            btrim(obligation) <> ''
            AND btrim(scope) <> ''
            AND btrim(basis) <> ''
        ),
    -- 关闭依据项的封闭三值（领域 ObligationItemState）。
    CONSTRAINT closure_obligation_item_state_closed
        CHECK (item_state IN ('CONCLUDED', 'HANDED_OVER', 'UNRESOLVED')),
    -- 承接项必须指名接收责任方（CONTEXT 硬句 219：发送交接、技术送达、部分接受或
    -- 默认超时接受都不能证明责任已经移交）；非承接项不得携带承接对象。
    CONSTRAINT closure_obligation_item_handover_named
        CHECK (
            (item_state = 'HANDED_OVER') = (handed_to IS NOT NULL)
            AND (handed_to IS NULL OR btrim(handed_to) <> '')
        ),
    CONSTRAINT closure_obligation_item_interval_ordered
        CHECK (applies_until IS NULL OR applies_until > applies_from)
);

CREATE TABLE customs_compliance.gate_condition_catalog (
    tenant_id    text        NOT NULL,
    scope_ref    text        NOT NULL,
    action       text        NOT NULL,
    boundary_ref text        NOT NULL,

    registered_at timestamptz NOT NULL,

    CONSTRAINT gate_condition_catalog_pkey
        PRIMARY KEY (tenant_id, scope_ref, action, boundary_ref),

    CONSTRAINT gate_condition_catalog_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(scope_ref) <> ''
            AND btrim(boundary_ref) <> ''
        ),
    -- 受门禁约束的方向性动作封闭四值（领域 GuardedAction）。门禁判断绑定动作与边界，
    -- 不能复用于其他动作（CONTEXT 硬句 216）——动作因此在主键里，不是一个附属列。
    CONSTRAINT gate_condition_catalog_action_closed
        CHECK (action IN (
            'OUTBOUND_RELEASE',
            'LOADING_DEPARTURE',
            'CROSS_CUSTOMS_MOVEMENT',
            'FINAL_DELIVERY'
        ))
);

CREATE TABLE customs_compliance.gate_condition_finding (
    tenant_id       text NOT NULL,
    scope_ref       text NOT NULL,
    action          text NOT NULL,
    boundary_ref    text NOT NULL,
    precondition_ref text NOT NULL,

    finding_state   text NOT NULL,

    CONSTRAINT gate_condition_finding_pkey
        PRIMARY KEY (tenant_id, scope_ref, action, boundary_ref, precondition_ref),
    CONSTRAINT gate_condition_finding_catalog_exists
        FOREIGN KEY (tenant_id, scope_ref, action, boundary_ref)
        REFERENCES customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref),

    CONSTRAINT gate_condition_finding_not_blank
        CHECK (btrim(precondition_ref) <> ''),
    -- 单项前置条件的封闭三值。刻意没有「未知」格：判断不出来的前置条件根本不该进
    -- 折叠（领域 PreconditionState 同一句），库内不给它落脚点。
    CONSTRAINT gate_condition_finding_state_closed
        CHECK (finding_state IN ('MET', 'UNMET', 'CONFLICTING'))
);
