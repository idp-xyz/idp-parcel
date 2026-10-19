-- 歧义信号的读法（ADR-0195）：身份核对按（租户，号码）问「此刻有没有一份`已提交`委托把这个号声明为客户包裹件号」。
--
-- 客户包裹件号只在委托快照里（ADR-0184 决定四：挂在委托上、首次提交时固定），不另建投影列：读口按包含查快照里那
-- 一组，写入方照旧只写快照，不多一处要与快照同笔写的投影。部分索引只收`已提交`的行（state=1 即 SUBMITTED）——信号
-- 只认这一格，委托一经接受、拒绝或撤回就出索引。取值镜像领域 ShipmentRequestState，编号的唯一来源是领域包，SQL 只
-- 比较不拥有编号。jsonb_path_ops 只支持包含（@>），读口正是按包含查。

CREATE INDEX shipment_request_submitted_customer_parcel_numbers
    ON parcel_shipment.shipment_request
    USING GIN ((snapshot -> 'customerParcelNumbers') jsonb_path_ops)
    WHERE state = 1;
