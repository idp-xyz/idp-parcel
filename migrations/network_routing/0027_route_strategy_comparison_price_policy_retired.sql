-- 路由策略版本的比较价格政策一列退场（ADR-0260 决定三，票 docs-consistency/code/25）。
--
-- 0016 让路由策略版本登一份「所引价格政策」，按 ADR-0148 决定四由它声明比较币种换算的汇率口径。ADR-0260 决定三把
-- 换算定为：所用汇率与口径就是各段价卡绑定的那串汇率序列，路由策略版本所引的价格政策不参与换算——这一列从此只能是
-- 一处登了却不起作用的声明，正是该记录要拆掉的那一种。比较币种一列不动，它仍是策略版本上的租户取值。
--
-- 删列，不留作历史列：这一列从来没有登记口能写——受控批量口与在线口共用的路由策略批文形状里没有这一键（严格解码拒
-- 未知键），也没有任何读方；留着它只多一格永远为空、又要另加 CHECK 拦新值的死列。万一有人绕开登记口直接写过 SQL，
-- 删列就会抹掉一份谁也没读过、却是某人当真写下的声明，所以有非空值就拒绝施加：导出那几行、由写它的一方决定去留后
-- 置空，再重跑本迁移。0016 的头注与 route_strategy_version_comparison_policy_not_blank 约束不改也不另删：已施加的
-- 迁移正文按校验和守着，约束随列一并消失。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM network_routing.route_strategy_version WHERE comparison_price_policy IS NOT NULL) THEN
        RAISE EXCEPTION USING MESSAGE =
            'network_routing.route_strategy_version 有登了 comparison_price_policy 的行：这一列不再参与换算（ADR-0260 决定三）。'
            || '请导出这些行备查，由登记它们的一方决定去留后置空，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE network_routing.route_strategy_version
    DROP COLUMN comparison_price_policy;
