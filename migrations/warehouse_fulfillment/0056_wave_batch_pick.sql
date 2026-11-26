-- 合并拣货：放波次时选的拣货方式。合并拣货的波次，拣货的人按库位、商品把几张出库请求的货合在一起拿，
-- 拿回来之后再按出库请求分开（分拣）；不选就是逐单拣，货在拣货时就按单分开。已有的波次都是逐单拣。
-- 账上的单位不变，仍是一条任务一笔：这一列只说拣货的人怎么走、拿回来的货要不要分拣。

ALTER TABLE warehouse_fulfillment.wave
    ADD COLUMN batch_pick boolean NOT NULL DEFAULT false;
