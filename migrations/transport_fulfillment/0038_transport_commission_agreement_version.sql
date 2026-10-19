-- 运输委托记下登记方指名的那一版协议：对象与版本标签两格（票 demo-chain-gaps/28，ADR-0207 文末补记）。
--
-- agreement_ref 存的是 party-commercial 一处拼出的「对象/版本」指称，本上下文照原样存、不拆（ADR-0080 决定七）。运输收费
-- 发生项从委托取采购上下文时，要按这份委托回问 PC 那一版协议此刻是否支持采购、声明了哪些收费发生条件，问口按（对象、版本
-- 标签）两格收，所以提交时把登记方指名的两格一并留下。两格只在 INSERT 里出现，取消、开始与前移的 UPDATE 不碰它们。
--
-- 可缺而不拒迁移：委托提交口已在生产上接通，表里可能已有行。那些行当时指名的两格只在内容摘要里，摘要不可逆；从快照串里
-- 拆出来就是在本上下文替 PC 解它的拼法。所以旧行两列留空，如实说「没记下」：从这样的委托取采购上下文时，登记方须自己给出
-- 协议对象与版本，由 PC 核过后按快照与委托比对（编排的头注）。两列要么都缺、要么都在且非空；只缺一列时 btrim 比较得 NULL，
-- CHECK 会把 NULL 当作通过，所以「都在」一支另写 IS NOT NULL。
ALTER TABLE transport_fulfillment.transport_commission
    ADD COLUMN agreement_object  text,
    ADD COLUMN agreement_version text;

ALTER TABLE transport_fulfillment.transport_commission
    ADD CONSTRAINT transport_commission_agreement_version_pair
        CHECK (
            (agreement_object IS NULL AND agreement_version IS NULL)
            OR (agreement_object IS NOT NULL AND agreement_version IS NOT NULL
                AND btrim(agreement_object) <> '' AND btrim(agreement_version) <> '')
        );
