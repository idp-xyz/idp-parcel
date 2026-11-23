-- 出库请求是否平台备货。没标的行保持按包裹各交一份委托；标了的等全部包裹打包后交一份。
-- false 是「没标」，不是租户取值。

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN platform_stocking boolean NOT NULL DEFAULT false;
