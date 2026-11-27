-- 拆出来的请求可以再拆。0025 的约束不许带着父请求的行成为已拆单：拆单的子请求、拆出缺货部分得到的那一张
-- 再走拆单时，把它记成已拆单的那一句违反约束、整笔回滚，调用方看到的是一个 500。
-- 拆出来的请求与受理进来的请求是同一种东西——它也会缺货、也会被货主要求分开发——没有不许再拆的道理。
-- 约束只留「父请求标识不是空白」这一半。

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_parent_shape;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_parent_shape CHECK (
        parent_request_id IS NULL OR btrim(parent_request_id) <> ''
    );
