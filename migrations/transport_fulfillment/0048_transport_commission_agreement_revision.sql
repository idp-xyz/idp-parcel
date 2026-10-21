-- 委托协议换版（ADR-0231 决定三、四；票 demo-chain-gaps/88）。
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
