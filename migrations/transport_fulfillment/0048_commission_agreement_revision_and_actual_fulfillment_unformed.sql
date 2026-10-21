-- 委托协议换版册与实际履约未形成册（ADR-0231；票 demo-chain-gaps/88）。两张表同一个决定立起来，同一份迁移落：换版册是补形成的一个触发，
-- 未形成册是补形成留下的痕。

-- 委托协议换版（ADR-0231 决定三、四）。
--
-- 一行是委托上的一次换版：自 applies_from 起这份委托改按同一协议对象的后继版本采购，快照与采购责任法人取自运营人员换版时核过的
-- 那一版。提交那一版仍在 transport_commission 那一行上（第 1 版），本表从第 2 版起，号在同一份委托内连续。事实按哪一版判，由起用
-- 时刻不晚于事实业务时间的各行里号最大的那一行定，没有就是提交那一版——这条读法只在领域一处（TransportCommission.AgreementTermsAt）。
--
-- 只插不改：换版之后不改、不删，旧版留作历史，起用之前的事实仍按它判；要纠正一次换错的版本就再换一次。主键（租户、委托、号）由编排
-- 在取到这份委托的换版锁之后按「已有几版 + 1」铸出；依据（换版所凭的那一份运营决定）在同一份委托内唯一，同一份决定重放落回原行。
-- 外键钉「委托须已提交」，编排已在落行之前读过委托，这里是第二道（0001 立的纪律）。
--
-- agreement_ref 照 transport_commission 那一列的规矩存 PC 一处拼出的指称、不拆；successor_object 与 successor_version 是运营人员指名、
-- 回问 PC 用的那两格（同 0038 的理由）。
CREATE TABLE transport_fulfillment.transport_commission_agreement_revision (
    tenant_id                    text        NOT NULL,
    commission_id                text        NOT NULL,
    revision                     integer     NOT NULL,

    successor_object             text        NOT NULL,
    successor_version            text        NOT NULL,
    agreement_ref                text        NOT NULL,
    procurement_legal_entity_ref text        NOT NULL,
    applies_from                 timestamptz NOT NULL,
    basis                        text        NOT NULL,
    decided_by                   text        NOT NULL,
    registered_at                timestamptz NOT NULL,
    inserted_at                  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT transport_commission_agreement_revision_pkey
        PRIMARY KEY (tenant_id, commission_id, revision),

    CONSTRAINT transport_commission_agreement_revision_basis_unique
        UNIQUE (tenant_id, commission_id, basis),

    CONSTRAINT transport_commission_agreement_revision_commission_fkey
        FOREIGN KEY (tenant_id, commission_id)
        REFERENCES transport_fulfillment.transport_commission (tenant_id, commission_id),

    -- 第 1 版是提交那一版，不在本表。
    CONSTRAINT transport_commission_agreement_revision_number_after_submission
        CHECK (revision >= 2),

    CONSTRAINT transport_commission_agreement_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(commission_id) <> ''
            AND btrim(successor_object) <> ''
            AND btrim(successor_version) <> ''
            AND btrim(agreement_ref) <> ''
            AND btrim(procurement_legal_entity_ref) <> ''
            AND btrim(basis) <> ''
            AND btrim(decided_by) <> ''
        )
);

-- 实际履约未形成册（ADR-0231 决定六）。
--
-- 已采用委托的段上，一个对象的参与以下一次权威交接或有效交付结束，自动形成那一路（或采用、换版之后的补形成）把它交给登记用例，登记
-- 用例答了「这一次不形成」：委托那一版没声明实际履约的收费发生条件（未配置）、那一版在参与终点未生效或已终止（截止与退役之后就落这两
-- 格，ADR-0233），或委托提交时没记下协议版本（缺采购上下文）。哪个段里的哪个对象、在哪份委托下按第几版、为什么、问的是哪一版协议、
-- 首次与最近一次答复时刻，记在这里；之后某一次形成了、或认出已有一项发生项盖住它，了结为已形成；那一版改说不计费，了结为不计费。
-- 写它的只有补形成那一处（application.FormActualFulfillmentOnParticipationEndHandler），与那一拍同笔：派发进程里就是 inbox 那一笔。
--
-- **一个（租户，段，对象）一行。** 同一段同一对象只有一条参与替代链（再次进入是新的段），一条参与只结束一次。
--
-- **本表是待办台账，不是事实来源。** 一行随最近一次答复改写状态：参与、委托与它的换版、发生项各在各的册上，本表不替它们留历史。
-- 不计费那一格只在先前答过「未形成」的那一条上了结，从没进过册的不计费不进册：那是协议的如实答案，进册就会把每一段按协议本不收费的
-- 参与都列成待办。
CREATE TABLE transport_fulfillment.actual_fulfillment_unformed (
    tenant_id          text        NOT NULL,
    segment_ref        text        NOT NULL,
    object_ref         text        NOT NULL,

    -- 最近一次答复时这一段采用的委托，与那一刻作答的那一版在委托上的号（1 是提交那一版）。
    commission_id      text        NOT NULL,
    commission_revision integer    NOT NULL,
    -- 参与终点的业务时间：发生项按它核那一版协议。
    ended_at           timestamptz NOT NULL,
    -- 最近一次答「未形成」时的那一格。了结之后照留：那是这一件为什么等过。
    reason             text        NOT NULL,
    -- 最近一次答「未形成」时去问 PC 的那一版；缺采购上下文那一格没问，两列同缺。
    agreement_object   text,
    agreement_version  text,
    first_answered_at  timestamptz NOT NULL,
    last_answered_at   timestamptz NOT NULL,

    state              text        NOT NULL,
    -- 已形成时盖住它的那一项发生项（引用与有效性版本）。
    occurrence_ref     text,
    occurrence_version text,
    -- 不计费时作答的那一版协议快照。
    not_charged_by     text,
    settled_at         timestamptz,

    CONSTRAINT actual_fulfillment_unformed_pkey
        PRIMARY KEY (tenant_id, segment_ref, object_ref),

    CONSTRAINT actual_fulfillment_unformed_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(segment_ref) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(commission_id) <> ''
        ),

    CONSTRAINT actual_fulfillment_unformed_revision_positive
        CHECK (commission_revision >= 1),

    CONSTRAINT actual_fulfillment_unformed_reason_closed
        CHECK (reason IN ('NOT_CONFIGURED', 'AGREEMENT_NOT_EFFECTIVE', 'AGREEMENT_TERMINATED', 'PROCUREMENT_CONTEXT_MISSING')),

    -- 问过的那一版两格同在或同缺；缺采购上下文没问 PC，必缺，其余三格必在。
    CONSTRAINT actual_fulfillment_unformed_agreement_coupled
        CHECK (
            (reason = 'PROCUREMENT_CONTEXT_MISSING' AND agreement_object IS NULL AND agreement_version IS NULL)
            OR (reason <> 'PROCUREMENT_CONTEXT_MISSING' AND agreement_object IS NOT NULL AND agreement_version IS NOT NULL
                AND btrim(agreement_object) <> '' AND btrim(agreement_version) <> '')
        ),

    CONSTRAINT actual_fulfillment_unformed_state_closed
        CHECK (state IN ('UNFORMED', 'FORMED', 'NOT_CHARGED')),

    -- 状态与了结各格成组：未形成没有了结；已形成必有盖住它的那一项、没有不计费格；不计费必有作答的快照、没有发生项。
    CONSTRAINT actual_fulfillment_unformed_settlement_coupled
        CHECK (
            (state = 'UNFORMED' AND occurrence_ref IS NULL AND occurrence_version IS NULL AND not_charged_by IS NULL AND settled_at IS NULL)
            OR (state = 'FORMED' AND occurrence_ref IS NOT NULL AND btrim(occurrence_ref) <> ''
                AND occurrence_version IS NOT NULL AND btrim(occurrence_version) <> ''
                AND not_charged_by IS NULL AND settled_at IS NOT NULL)
            OR (state = 'NOT_CHARGED' AND not_charged_by IS NOT NULL AND btrim(not_charged_by) <> ''
                AND occurrence_ref IS NULL AND occurrence_version IS NULL AND settled_at IS NOT NULL)
        )
);

-- 按租户列未形成的那几件：查阅页只问 UNFORMED，参与终点最早的在前。
CREATE INDEX actual_fulfillment_unformed_open_by_tenant
    ON transport_fulfillment.actual_fulfillment_unformed (tenant_id, ended_at, segment_ref, object_ref)
    WHERE state = 'UNFORMED';
