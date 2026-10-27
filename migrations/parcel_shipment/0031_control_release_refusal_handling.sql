-- 释放未受理册的处理留痕（票 docs-consistency/code/95；ADR-0293）。
--
-- control_release_refusal_handling 记下人对册上一笔被拒收的占用做过的处理：谁、用哪一种做法、凭什么、何时。只追加：册上原行不改，
-- 处理留痕也不改不删——册上的一行是否「未处理」由有没有对应的处理行决定，不在原行上加状态列。主键就是册行的主键（租户，委托，
-- 提交版本，控制结果），一笔占用至多被处理一次；两人同时处理，主键相撞，只有一笔成立，败者读回赢家。外键指向册行，处理留痕
-- 不会挂在一个册上没有的占用上。
--
-- handling_kind 是封闭两值：RELEASED_AT_SETTLEMENT（核对后经按原关联的释放编排放掉，提供方答已收口才落）与 MARKED_HANDLED（资金
-- 已在本产品之外处置，只留痕）。handled_by 是（发行方，sub）这一对的引用，handling_basis 是处理人核对后留下的依据，两样必填。
--
-- control_release_refusal_tenant_recorded 服务运营查阅的 ORDER BY recorded_at LIMIT：主键前缀按（租户，委托）排，按租户取最早登记
-- 的未处理行用不上它。
--
-- 全部 CHECK 走 IS NULL / IS NOT NULL 缝（f822e1d 入册的三值逻辑纪律）；本表的列都 NOT NULL，没有三值问题。

CREATE TABLE parcel_shipment.control_release_refusal_handling (
    tenant_id           text        NOT NULL,
    shipment_request_id text        NOT NULL,
    submission_version  text        NOT NULL,
    control_result_id   text        NOT NULL,

    handling_kind       text        NOT NULL,
    handled_by          text        NOT NULL,
    handling_basis      text        NOT NULL,
    handled_at          timestamptz NOT NULL,

    CONSTRAINT control_release_refusal_handling_pkey
        PRIMARY KEY (tenant_id, shipment_request_id, submission_version, control_result_id),

    CONSTRAINT control_release_refusal_handling_refusal_fkey
        FOREIGN KEY (tenant_id, shipment_request_id, submission_version, control_result_id)
        REFERENCES parcel_shipment.control_release_refusal
            (tenant_id, shipment_request_id, submission_version, control_result_id),

    CONSTRAINT control_release_refusal_handling_kind_closed
        CHECK (handling_kind IN ('RELEASED_AT_SETTLEMENT', 'MARKED_HANDLED')),

    CONSTRAINT control_release_refusal_handling_not_blank
        CHECK (
            btrim(handled_by) <> ''
            AND btrim(handling_basis) <> ''
        )
);

CREATE INDEX control_release_refusal_tenant_recorded
    ON parcel_shipment.control_release_refusal (tenant_id, recorded_at, shipment_request_id);
