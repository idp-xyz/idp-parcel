-- 网络定义登记册：只登记「这个范围有没有网络定义、当前修订是哪一版」（ADR-0053）。
--
-- 它**不存事实族**。服务区解析、路径可执行性、分值那九族以每次判断生成的候选为轴，
-- 是解析层的推导结果而不是目录行；判断键里连目的地都没有那一维，按九族建表等于给一次
-- 尚未发生的判断预存结果（ADR-0053 Context 的三条硬证据）。
--
-- 它也**不存定义原语**。服务区及其版本、排班、拓扑、策略、排序准则、日历与缓冲的模式，
-- 按 ADR-0053 第三条留待真有一份定义可登时再设计——没有真实定义在手，列是替租户拟的，
-- 而模式一旦落库就按校验和固定。
--
-- 于是本表今天没有写入方，读口因此恒答`未配置`。那不是遗漏：解析层属 PAR-NET-14，
-- 在它到位之前任何非空答复都是编的（ADR-0052 增设`未配置`格正是为了让这句话说得出口）。
--
-- view_revision 与定义同行：ADR-0052 要求「事实与修订一次取回、同次取回共用一版」，
-- 修订因此必须由登记册派生而不是调用方传入。

CREATE TABLE network_routing.network_definition (
    tenant_id       text        NOT NULL,
    service_purpose text        NOT NULL,

    view_revision   text        NOT NULL,
    registered_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT network_definition_pkey
        PRIMARY KEY (tenant_id, service_purpose),

    CONSTRAINT network_definition_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(service_purpose) <> ''
            AND btrim(view_revision) <> ''
        )
);
