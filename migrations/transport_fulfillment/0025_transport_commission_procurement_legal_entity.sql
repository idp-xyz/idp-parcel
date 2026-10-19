-- 运输委托加采购责任法人一格（票 demo-chain-gaps/23，ADR-0202 决定三）。
--
-- 法人在提交时从核过的那一版供应商协议快照下来，不由登记方给；与协议、条件、角色、责任四格同样只在 INSERT 里出现，
-- 取消与开始的 UPDATE 不碰它。
--
-- 委托表在提交口接线之前没有任何写入方，应当没有行。有行就拒绝迁移，不替任何人补一个法人——那一行当时采用的是哪一版
-- 协议、由哪个法人承担采购责任，只有回 party-commercial 按那一版核过才答得出，不能在这里猜一个默认值（同 0024 的做法）。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.transport_commission) THEN
        RAISE EXCEPTION USING MESSAGE =
            'transport_commission 已有行，不替它们猜采购责任法人：请导出这些行备查，由提交它们的一方按所采用的供应商协议'
            || '版本重新提交后清空本表，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE transport_fulfillment.transport_commission
    ADD COLUMN procurement_legal_entity_ref text NOT NULL;

ALTER TABLE transport_fulfillment.transport_commission
    ADD CONSTRAINT transport_commission_legal_entity_not_blank
        CHECK (btrim(procurement_legal_entity_ref) <> '');
