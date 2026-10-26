-- 网络连接代码不得含窗口依据的分隔符 `+`（票 demo-chain-gaps/176）。
--
-- 计划段的窗口依据写作 `CONNECTION/<代码>@<版本>`，各格之间以 `+` 相连；TF 的运输准备按 `+` 切格、认 `CONNECTION/`
-- 前缀、要求恰一格，再在最后一个 `@` 处切掉版本（ADR-0201 决定二）。代码里带 `+`，那一格就被切成两半，准备那一路在
-- 每封初始路由信封上失败。登记用例的受理门同格拒（CONNECTION_CODE_CARRIES_BASIS_SEPARATOR），这一条是库上的第二道网。
-- `@` 与 `/` 不在此列：版本在最后一个 `@` 之后且只由数字组成，前缀只认一次，带它们的代码照样读回原样。
--
-- 只收连接代码一列。线路段链、日历与调整里的对象身份不收：带 `+` 的连接登不进来，引它的就是引一条没登的连接，与引用
-- 别的没登的连接同一种落法。节点代码不进节点间段的窗口依据，交付段 TF 不读。
--
-- 已有带 `+` 的行就拒绝施加并列出它们，不替登记方改名：连接代码是 TF 班次方向的一格，改了名要连同引用它的线路段、
-- 日历、调整与班次方向一起换，由登记它们的一方决定。目录只增不改，换法是另起一个不带 `+` 的代码重登，再移走原行后
-- 重跑本迁移。
DO $$
DECLARE
    offending text;
BEGIN
    SELECT string_agg(format('%s/%s@%s', tenant_id, connection_code, version), ', '
                      ORDER BY tenant_id, connection_code, version)
      INTO offending
      FROM network_routing.network_connection_version
     WHERE strpos(connection_code, '+') > 0;
    IF offending IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'network_routing.network_connection_version 有连接代码含窗口依据分隔符 + 的行（租户/代码@版本）：' || offending
            || '。TF 运输准备读不回这些连接（ADR-0201 决定二）。请由登记它们的一方换不带 + 的代码重登并移走原行，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE network_routing.network_connection_version
    ADD CONSTRAINT network_connection_version_code_without_basis_separator
        CHECK (strpos(connection_code, '+') = 0);
