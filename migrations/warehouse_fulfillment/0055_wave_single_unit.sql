-- 单品单件波次：波次里每一张出库请求要拣的都只有一件（预占需求合计是 1 个基本单位）。
-- 这种波次拣货时同一库位同一种货合成一格拿、复核打包时扫货就能配单，所以放出时把它记在波次上，
-- 读的时候不必再去数每张请求的预占。已有的波次按同一条规则回填。

ALTER TABLE warehouse_fulfillment.wave
    ADD COLUMN single_unit boolean NOT NULL DEFAULT false;

UPDATE warehouse_fulfillment.wave AS target
   SET single_unit = true
 WHERE EXISTS (
        SELECT 1 FROM warehouse_fulfillment.wave_request member
         WHERE member.tenant_id = target.tenant_id AND member.wave_id = target.wave_id)
   AND NOT EXISTS (
        SELECT 1 FROM warehouse_fulfillment.wave_request member
         WHERE member.tenant_id = target.tenant_id AND member.wave_id = target.wave_id
           AND (SELECT coalesce(sum(reservation.wanted), 0)
                  FROM warehouse_fulfillment.outbound_reservation reservation
                 WHERE reservation.tenant_id = member.tenant_id
                   AND reservation.request_id = member.request_id) <> 1);
