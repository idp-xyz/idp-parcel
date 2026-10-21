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
