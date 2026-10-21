-- 发生项失效的生产入口（ADR-0232 决定四、五；票 demo-chain-gaps/57）。
--
-- 修订者：经修订入口长出的每一版（失效，或失效之后凭同一件事实重新成立的替代版本）记下作出那一次决定的人，写法与登记者同一个
-- 「发行方#sub」。可空：此前直接经 ReviseValidity 形成的修订版本不带它，首版也不带——首版的出处是发生条件上的登记者。
--
-- 一版至多被修订一次：有效性版本链是一条线，不分叉。新版本号由前一版派生（V1、V2……），两笔并发修订同一版本就撞主键；这条唯一
-- 索引另把「另起一个号回指同一版」也挡在库面，链头因此只有一个，按 corrects_version 找后继的各处（覆盖一问、按范围列链头）读到的
-- 永远是同一条链。
ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD COLUMN revised_by text;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_revised_by_on_revision
        CHECK (revised_by IS NULL OR (revision_kind IS NOT NULL AND btrim(revised_by) <> ''));

CREATE UNIQUE INDEX transport_charge_occurrence_corrected_once
    ON transport_fulfillment.transport_charge_occurrence (tenant_id, occurrence_ref, corrects_version)
 WHERE corrects_version IS NOT NULL;

-- 解除履约段采用（ADR-0232 决定一至三；票 demo-chain-gaps/57）。
--
-- 采用册仍只插不改：解除不改写也不删掉那一次采用，而是另记一行解除；解除之后同一段可以再采用（指错了委托，解除之后采用对的那一份），
-- 再采用是这一段的下一代采用。主键因此从（租户、段）扩成（租户、段、代次）：既有的行都是第一代。
--
-- 一段此刻至多一份当前采用（没有解除的那一代），由两道约束一起守：第二代起必须指名上一代（previous_generation = generation - 1），
-- 上一代必须已有解除（外键指向解除册）。于是一段上的各代排成一条线，只有最后一代可能没有解除。
ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD COLUMN generation integer NOT NULL DEFAULT 1,
    ADD COLUMN previous_generation integer;

ALTER TABLE transport_fulfillment.segment_commission_adoption
    DROP CONSTRAINT segment_commission_adoption_pkey;

ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD CONSTRAINT segment_commission_adoption_pkey
        PRIMARY KEY (tenant_id, segment_ref, generation);

ALTER TABLE transport_fulfillment.segment_commission_adoption
    ALTER COLUMN generation DROP DEFAULT;

ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD CONSTRAINT segment_commission_adoption_generation_chained
        CHECK (
            (generation = 1 AND previous_generation IS NULL)
            OR (generation > 1 AND previous_generation = generation - 1)
        );

-- 一行是对一段的一代采用的一次解除：谁解的、解的是哪一份委托，以及委托的开始因此怎么变——解除之前的开始、变法（撤回到已提交、
-- 按其余已采用段最早的首个参与重新派生、或不动）与之后的开始。原开始留在采用那一行，委托的开始换了什么、凭哪一次解除换的留在这一行，
-- 委托那一行只是此刻的样子（ADR-0232 决定二、三）。只插不改。
CREATE TABLE transport_fulfillment.segment_commission_adoption_withdrawal (
    tenant_id          text        NOT NULL,
    segment_ref        text        NOT NULL,
    generation         integer     NOT NULL,

    commission_id      text        NOT NULL,
    withdrawn_by       text        NOT NULL,
    start_before_basis text        NOT NULL,
    start_before_at    timestamptz NOT NULL,
    start_change       text        NOT NULL,
    start_after_basis  text,
    start_after_at     timestamptz,
    recorded_at        timestamptz NOT NULL,
    inserted_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT segment_commission_adoption_withdrawal_pkey
        PRIMARY KEY (tenant_id, segment_ref, generation),

    CONSTRAINT segment_commission_adoption_withdrawal_adoption_fkey
        FOREIGN KEY (tenant_id, segment_ref, generation)
        REFERENCES transport_fulfillment.segment_commission_adoption (tenant_id, segment_ref, generation),

    CONSTRAINT segment_commission_adoption_withdrawal_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(segment_ref) <> ''
            AND btrim(commission_id) <> ''
            AND btrim(withdrawn_by) <> ''
            AND btrim(start_before_basis) <> ''
        ),

    CONSTRAINT segment_commission_adoption_withdrawal_change_closed
        CHECK (start_change IN ('WITHDRAWN', 'REDERIVED', 'KEPT')),

    -- 撤回到已提交没有之后的开始；重新派生与不动都有，不动的那一格之后等于之前。
    CONSTRAINT segment_commission_adoption_withdrawal_after_coupled
        CHECK (
            (start_change = 'WITHDRAWN' AND start_after_basis IS NULL AND start_after_at IS NULL)
            OR (start_change = 'REDERIVED' AND start_after_basis IS NOT NULL AND btrim(start_after_basis) <> ''
                AND start_after_at IS NOT NULL)
            OR (start_change = 'KEPT' AND start_after_basis = start_before_basis AND start_after_at = start_before_at)
        )
);

ALTER TABLE transport_fulfillment.segment_commission_adoption
    ADD CONSTRAINT segment_commission_adoption_previous_withdrawn
        FOREIGN KEY (tenant_id, segment_ref, previous_generation)
        REFERENCES transport_fulfillment.segment_commission_adoption_withdrawal (tenant_id, segment_ref, generation);
