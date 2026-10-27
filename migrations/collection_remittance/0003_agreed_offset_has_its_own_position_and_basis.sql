-- 经 settlement-accounting 认可的协议抵扣在代收分户账上有了记账形（ADR-0298）：认可的
-- 抵扣单独登记一册（agreed_offset），落账凭它把本金自渠道在途移到已抵扣这一账内终局
-- 位置；一笔记账至多被冲正一次。与领域构造门 domain.RecordSubledgerPosting 是同一份
-- 规则的两处落点，两处同笔改。
--
-- 本迁移只放开词表、加约束与索引，不改任何已落行。旧口径下已落的行都满足新增各条，
-- 唯一的例外是同一笔记账被冲正过两次以上——那样的账 CREATE UNIQUE INDEX 会在施加时
-- 失败、整笔迁移回滚，那正是要人回头看的账：哪一笔冲正是多余的，库自己判不了，不能
-- 替它挑一笔删掉。演示租户的存量没有冲正，满足全部各条。

-- 认可的协议抵扣：settlement-accounting 认可、本上下文只引用的一笔。认可及其金额归
-- 它，这里保存当时引用的那一份；不据此形成任何运营应收应付。登记只钉引用到一条指令
-- 上，不动账——落账是另一笔以本行为依据的记账，分户账键与按指令的上限因此由依据推出。
-- 追加式：没有更新与删除路径，认可被更正走新认可新行，被撤回走冲正已落的记账。
CREATE TABLE collection_remittance.agreed_offset (
    tenant_id       text        NOT NULL,
    offset_ref      text        NOT NULL,

    instruction_ref text        NOT NULL,
    currency        text        NOT NULL,
    amount_minor    bigint      NOT NULL,
    approval_ref    text        NOT NULL,
    approved_at     timestamptz NOT NULL,

    CONSTRAINT agreed_offset_pkey
        PRIMARY KEY (tenant_id, offset_ref),

    CONSTRAINT agreed_offset_instruction_fkey
        FOREIGN KEY (tenant_id, instruction_ref)
        REFERENCES collection_remittance.collection_instruction (tenant_id, instruction_ref),

    CONSTRAINT agreed_offset_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(offset_ref) <> ''
            AND btrim(instruction_ref) <> ''
            AND btrim(currency) <> ''
            AND btrim(approval_ref) <> ''
        ),

    CONSTRAINT agreed_offset_amount_positive
        CHECK (amount_minor > 0)
);

-- 同一份认可落到同一条指令上至多一行：同一笔抵扣不能凭两条记录落两次。同一份认可引用
-- 落到不同指令上是不同的行——一次回款的抵扣常按指令拆开。写口的 ON CONFLICT DO NOTHING
-- 不带冲突目标，撞上这条同样折成`已登记`，编排按标识读不回同一条，答内容冲突。
CREATE UNIQUE INDEX agreed_offset_one_per_approval_and_instruction
    ON collection_remittance.agreed_offset (tenant_id, approval_ref, instruction_ref);

-- 词表放开：已抵扣是账内位置，AGREED_OFFSET 是新增的一种依据。去向侧仍不含 EXTERNAL_SOURCE，
-- 已抵扣与已汇付一样是账内终局，全账总额恒等于入账之和。
ALTER TABLE collection_remittance.subledger_posting
    DROP CONSTRAINT subledger_posting_from_position_closed,
    DROP CONSTRAINT subledger_posting_to_position_closed,
    DROP CONSTRAINT subledger_posting_basis_kind_closed;

ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_from_position_closed
        CHECK (from_position IN (
            'EXTERNAL_SOURCE',
            'IN_TRANSIT_AT_CHANNEL',
            'AWAITING_ALLOCATION',
            'PAYABLE_TO_CUSTOMER',
            'REMITTED',
            'SHORTFALL',
            'SURPLUS',
            'OFFSET_APPLIED'
        )),
    ADD CONSTRAINT subledger_posting_to_position_closed
        CHECK (to_position IN (
            'IN_TRANSIT_AT_CHANNEL',
            'AWAITING_ALLOCATION',
            'PAYABLE_TO_CUSTOMER',
            'REMITTED',
            'SHORTFALL',
            'SURPLUS',
            'OFFSET_APPLIED'
        )),
    ADD CONSTRAINT subledger_posting_basis_kind_closed
        CHECK (basis_kind IN (
            'COLLECTION_FACT',
            'ALLOCATION',
            'REMITTANCE_BATCH',
            'DISCREPANCY',
            'CORRECTION',
            'AGREED_OFFSET'
        ));

-- 已抵扣的门：进入它必须凭认可的协议抵扣、且只从渠道在途来；认可的协议抵扣作依据只有
-- 这一种形；离开它只有冲正。三条合起来，已抵扣里的每一分钱都追溯到一份认可，也只能
-- 因这份认可被撤回而退回渠道在途。
ALTER TABLE collection_remittance.subledger_posting
    ADD CONSTRAINT subledger_posting_offset_needs_agreed_offset
        CHECK (
            to_position <> 'OFFSET_APPLIED'
            OR (basis_kind = 'AGREED_OFFSET' AND from_position = 'IN_TRANSIT_AT_CHANNEL')
        ),
    ADD CONSTRAINT subledger_posting_agreed_offset_shape
        CHECK (
            basis_kind <> 'AGREED_OFFSET'
            OR (from_position = 'IN_TRANSIT_AT_CHANNEL' AND to_position = 'OFFSET_APPLIED')
        ),
    ADD CONSTRAINT subledger_posting_offset_leaves_only_by_correction
        CHECK (from_position <> 'OFFSET_APPLIED' OR basis_kind = 'CORRECTION');

-- 一条认可的抵扣至多支撑一笔记账。冲正回渠道在途之后这一笔仍在账上，所以撤回的认可
-- 不能再落第二次；要落走新的认可。
CREATE UNIQUE INDEX subledger_posting_one_per_agreed_offset
    ON collection_remittance.subledger_posting (tenant_id, basis_ref)
    WHERE basis_kind = 'AGREED_OFFSET';

-- 一笔记账至多被冲正一次。冲正的依据是被冲正的记账，两笔冲正指向同一笔，第二笔就是
-- 从别处多拿：已抵扣、待清分这些位置混着同账别的指令的钱，余额够扣不等于那是这一笔
-- 该退的。冲正错了走冲正那一笔冲正，是另一笔的依据，不受此限。
CREATE UNIQUE INDEX subledger_posting_one_correction_per_original
    ON collection_remittance.subledger_posting (tenant_id, basis_ref)
    WHERE basis_kind = 'CORRECTION';
