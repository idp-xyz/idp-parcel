-- 面单渠道服务的包裹终局（票 label-channel-service-first-release/11）。
--
-- 两件：
--
-- 一、final_outcome 的责任结果封闭集加两值。UC-PS-004 终局来源责任矩阵加了面单渠道服务两行
-- （CONTEXT 生命周期节的两种产物）：LABEL_SERVICE_OUTCOME（面单渠道服务非取消终局结果——实际
-- 承运商首次有效收寄，或受控关闭下成功结果均已作废/不可逆失效）与 LABEL_SERVICE_FAILURE（终局
-- 失败结果——受控关闭下全部相关面单交易均已定案为明确失败）。两种服务形态的产物都叫「终局
-- 服务结果」，落同一张表：委托完成派生、取消前核验与继续尝试判断只认这一处当前有效终局。
-- 领域侧 ResponsibilityOutcomeKind 与本 CHECK 是同一个封闭集合的两份，同笔改。
--
-- 二、按包裹取全部相关面单交易的读口（ListByCoveredParcel）走快照文档的 coveredParcels 数组
-- 包含查，GIN 索引钉在那一段上——跨交易判断每次都要整册取，不能全表扫。

ALTER TABLE parcel_shipment.final_outcome
    DROP CONSTRAINT final_outcome_kind_closed;

ALTER TABLE parcel_shipment.final_outcome
    ADD CONSTRAINT final_outcome_kind_closed
        CHECK (outcome_kind IN
            ('EFFECTIVE_DELIVERY', 'RETURN_COMPLETED',
             'SERVICE_TERMINATED', 'REGULATORY_DISPOSITION_EXECUTED',
             'LABEL_SERVICE_OUTCOME', 'LABEL_SERVICE_FAILURE'));

CREATE INDEX label_transaction_by_covered_parcel
    ON parcel_shipment.label_transaction
    USING GIN ((snapshot -> 'coveredParcels') jsonb_path_ops);
