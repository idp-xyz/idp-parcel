-- 合同责任范围声明归属一个货主客户账户（票 first-party-api/09；demo-chain-gaps/102）。
--
-- 首方索赔口的信封账户只能核到「这份合同是不是登在我名下」，此前声明只按（租户+合同范围）登记，别的账户的合同引用也会答
-- 「声明在场」。归属加成一列，资格规则读口按账户过滤，别的账户的合同答「声明不在场」，与不存在同格，不泄露存在。
--
-- 键不变：（租户+合同范围）仍是主键，一份合同范围只属于一个账户；覆盖类型表的外键因此不动。
--
-- 存量行推不出归属：合同范围引用是登记时的自由文本，库里没有任何一处把它连到账户。猜一个账户等于替租户拍板谁的合同，
-- 所以有存量声明就拒绝施加，列出这些行；由租户运营按账户重登（`parcel-ve-register` 的索赔资格声明批文现要 `customer`）后
-- 删去原行，再重跑本迁移。演示种子不登这张表，空库不受影响。
DO $$
DECLARE
    stranded text;
BEGIN
    SELECT string_agg(tenant_id || '/' || contract_scope_ref, ', ' ORDER BY tenant_id, contract_scope_ref)
      INTO stranded
      FROM visibility_exception.claim_contract_scope;
    IF stranded IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            '合同责任范围声明表里有尚未指明货主客户账户的存量行（租户/合同范围）：' || stranded
            || '。请导出备查，按账户带 customer 重登后删去原行，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE visibility_exception.claim_contract_scope
    ADD COLUMN customer_ref text NOT NULL,
    ADD CONSTRAINT claim_contract_scope_customer_not_blank CHECK (btrim(customer_ref) <> '');
