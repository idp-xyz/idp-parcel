-- 终局版本在包裹内唯一（票 demo-chain-gaps/103）。
--
-- 重派生的终局只以终局版本（prior_version）指回前一代，前一代的采用键只在它自己那一行上：追踪投影要把这层前后
-- 关系译成同一事实引用下的来源事实替代，就按终局版本找回那一行（FinalOutcomes.FindByFinalVersion）；重派生翻旧
-- 那一步同样按终局版本认前版。两处都假定一个终局版本只落在一行上。签发器全局唯一铸号让它今天成立，这里把它钉进
-- 库面，读口不必去信任签发器——两行同号时 QueryRow 只会静默取其一。不采用记录没有终局版本，部分索引不收。
CREATE UNIQUE INDEX final_outcome_version_unique
    ON parcel_shipment.final_outcome (tenant_id, parcel_id, final_version)
    WHERE final_version IS NOT NULL;
