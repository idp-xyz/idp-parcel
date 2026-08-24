# 02 投影读口与端点施工(MCP-3,blocked on 01)

Category: enhancement
Status: open

地盘、阻塞边与纪律见 ../spec.md;端点形状与作用域模型以票 01 落库的 ADR 为准。开工条件:票 01 报出 SHA。等待期允许只读预研,不落笔不占号。

## 活

1. **ports**(internal/visibilityexception/ports):为运营查阅补列表读面——扩 `ProjectionStore` 或新立运营读端口,取舍按 ADR;键无客户维但含租户维(投影是租户内部对象)。若拓宽既有接口签名,按 parallel-sessions.md「工作树也会被阻断」选三步法或隔离 worktree,开工前频道广播。
2. **adapters/http**:新查询端点(路径按 ADR,如 GET /tracking-projections):
   - outcome 按运营语义设计,如实区分「无投影」与「未授权」,不背客户面 VIEW_NOT_FOUND 的探针合并义务(那是 ADR-0029 对外部客户的纪律,不适用租户内部运营面);
   - QueryIntake 同属 PAR-INT-01 待提供:本包不带任何实现(包括开发用采信头部版),装配点交 UnconfiguredIntake,一律 403 + ACCESS_CHANNEL_NOT_CONFIGURED(ADR-0055);
   - 内容形状转写投影的并行维度原词(物流/位置或控制/关务/交付退运/异常影响/ETA/可见性缺口/投影版本),不是客户视图的删减 body。
3. **adapters/postgres**:投影列表读适配器;真库测试实跑,-v 下 PASS 非 SKIP(DSN 见 parallel-sessions.md)。
4. **cmd/parcel-api**:装配四文件占号广播(声明接手原 MCP-4 失效占号)→ 端点挂载、端点表登记、装配测试 → 释号。
5. **验证**:全仓 gofmt / go vet / go test -count=1;真库用例单跑 -v 出示 PASS。

## 完成标准

端点在提交态可服务(403 未配置档,与委托查阅同深度),装配测试对真库实跑 PASS,send_to_session 1 报 SHA;票 03 以该 SHA 为开工条件。
