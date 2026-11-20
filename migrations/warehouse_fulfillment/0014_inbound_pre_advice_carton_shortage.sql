-- 整箱完结的少收按箱。商品少收仍是商品编码与基本单位数量；整箱少收是箱号，数量恒为一箱。
-- 一行恰好带商品编码或箱号之一。已经落下的行都是商品少收，箱号留空。
--
-- 唯一约束把 NULL 看成互不相同，所以商品行的空箱号、箱行的空商品编码不会互相撞键。
-- 主体检查用 IS NULL 与 IS NOT NULL 写成布尔：两边都空时两个分支都是假，不会因为 NULL 比较被当成通过。
--
-- 本迁移不种任何行。仓配开关关着的部署也施加本迁移。

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ALTER COLUMN goods_code DROP NOT NULL;

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD COLUMN carton_number text;

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    DROP CONSTRAINT inbound_pre_advice_shortage_not_blank;

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD CONSTRAINT inbound_pre_advice_shortage_line_index
        CHECK (line_index >= 0);

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD CONSTRAINT inbound_pre_advice_shortage_subject
        CHECK (
            (
                goods_code IS NOT NULL
                AND carton_number IS NULL
                AND btrim(goods_code) <> ''
                AND quantity > 0
            )
            OR (
                goods_code IS NULL
                AND carton_number IS NOT NULL
                AND btrim(carton_number) <> ''
                AND quantity = 1
            )
        );

ALTER TABLE warehouse_fulfillment.inbound_pre_advice_shortage
    ADD CONSTRAINT inbound_pre_advice_shortage_carton_key
        UNIQUE (tenant_id, pre_advice_id, carton_number);
