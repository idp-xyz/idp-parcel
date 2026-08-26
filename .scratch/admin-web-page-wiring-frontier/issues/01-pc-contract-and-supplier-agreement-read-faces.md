# PC 客户合同与供应商协议查阅面——同表同读法，两页一并接线

Category: enhancement
Status: resolved

自[接线前沿盘点](../report.md)批 A。这是二十三张骨架页里唯一一组**表、写入方、数据三样今天全在**的，缺的只有读面。

## 事实（实读代码与演示库取证，锚 `7ce41e4`）

1. 两页的数据都在 `party_commercial.commercial_version`，靠 `object_kind` 分：客户合同是 `CustomerContractObject`，供应商协议是 `SupplierAgreementObject`。这张表与已接线的服务产品页是同一张。
2. 写入方 `parcel-commercial publish` 已经支持这两类——`commercialKindFrom` 的封闭集镜像 `CommercialObjectKind` 全九类，不是子集。
3. 客户合同**演示库里已经有一版**，正文册 `customer_contract_content` 与控制绑定 `customer_contract_control_binding` 都有行（种子 `commercial/publish-batch.json` 发的）。供应商协议那一类零行——种子没发，不是发不了。
4. 装载口缺：`OperationsCatalogue` 已有服务产品、接单规则包、接受前控制、价格政策、结算政策、时点锚策略六个 `List*`，没有这两类。
5. 端点缺：`/commercial-service-products` 与 `/commercial-policies` 两条已在，都不收这两类。

## 要做什么

按 `ADR-0077`（主数据目录查阅照运营查阅形状）与 `ADR-0078`（隔离读准入），照 `parcelpricing` 那份最完整的样板逐件裁：

- `internal/partycommercial/ports/` 加两个伴生列表读端口（**不拓宽既有写口**——扩写侧接口会拆全部写侧测试替身，`parcelpricing/ports/catalogue_read.go` 的文件注释记着这条风险的来历）。租户在方法签名上。
- `OperationsCatalogue` 加两个 `List*`：装载方向照 `ListServiceProducts`（按 `object_kind` 取版本壳，左连接各自正文册），**一条语句取回父子**——`ReadExecutor` 不保证两条语句同一快照。
- `adapters/http/` 加查询处理器；`isolated_read_intake.go` 与 `catalogue_intake.go` 已在，按现有 `CommercialCatalogueIntake` 复用，不新立一路准入。
- `cmd/parcel-api` 装配（**占号，见 spec**）。
- 种子补一版供应商协议（`seeds/commercial/` 内，`seed.sh` 加行属占号文件）。
- `apps/admin-web/src/pages/party/` 两页接真；`page-registry.tsx` 的 `liveIds` 各加一行（**只改自己那行，不动邻行**）。

## 一处待裁（本票内裁，不另开 ADR）

客户合同与供应商协议要并进 `/commercial-policies` 的 `kind` 分派，还是各立入口。现有分派收的是**策略**类（接单规则包、接受前控制、时点锚、价格政策、结算政策）；合同与协议不是策略，是**商业关系的载体**。倾向各立入口，理由是 `kind` 分派一旦收下非策略类，那个参数名就开始说谎，而端点路径是对外契约的一部分、日后改代价更大。裁决与理由写进实现的处理器文件注释。

## 完成标准

- 两页各自答 `200` + 非空册（供应商协议要在种子补版之后）；未启用隔离读准入时仍答 `403`，与既有六条查阅端点同签名。
- 真库测试钉住：按类别只取本类（不串 `object_kind`）、租户隔离、空册如实答空不折成未配置、`limit` 非正即拒。
- 全仓 `go test -count=1 ./...` 绿（**含真库**——单跑一个真库用例看 `-v` 下是 `PASS` 不是 `SKIP`）。
- 页面层取证：从 WSL 起 vite，Edge 无头取 DOM，两页见 `SYN` 数据、无未配置码（形状照 `product-story-and-demo/04` 那次）。

## 交付（MCP-2，2026-08-26）

四笔：`d69bbca` 端口＋装载口＋端点＋真库测试，`ed2bdab` cmd 装配与放行面＋种子补两版供应商协议，`3f5fad9` 两页接真＋`liveIds`。

**「一处待裁」照倾向裁了：各立入口。** 裁决与两层理由写在 `query_commercial_relations.go` 的文件注释。第二层理由是实现时才看清的，记在这里免得日后只剩第一层：`/commercial-policies` 那个 `kind` 分的**是「商业规则与策略」一页里的五个页签，不是商业对象的种类**。合同与协议是管理台上两张独立的页，照 `/commercial-service-products` 的先例各配一个入口；折进一个带 `kind` 的入口，会让「页面加一个页签」和「产品多一类目录」在对外契约上长成同一个动作。

**放行面这一格没另开 ADR。** `ADR-0078` 的 Decision 一列了八条端点，但正文自身把那份名单限定在「当前装配点上」，入格判据是那三条（只消费本上下文的存储读面且不触发判断/派生/披露、零持久化、作用域是运营侧的授权结果）。两条新端点满足同三条，直接进；`isolated_read_test.go` 的枚举注释已把这层写明——那份枚举与两态测试互为对照，漂了两个方向都有信号。

**读面比骨架多出一件、少了五件，都是如实的。**

- 多出的是 `contentRegistered`：**无正文行**与**有正文行零绑定**在「零绑定」上撞成同一个可观察签名，而恢复动作相反（前者去登记正文，后者无事可做）。0012 专门用父子两表表达这个区别，读面不能把它丢在装载口里。页面据它分出三说，不看数组长度。
- 少的五列是骨架期的货主客户账户、责任法人（合同页）与供应商、采购价格条件、结算条件（协议页）。0012 的正文表只有 `rule_package_id` 与 `declared_at`，版本壳上也不带客户或法人坐标；供应商协议根本没有正文册。五列一概不上，由页面说明交代缺的是**登记面**而非转写——照 README「列表页上列通则」第三条（服务区域地理覆盖列的先例）。

**取证（全部实跑，不是推断）**

1. 真库四个用例 `-v` 下 `PASS` 非 `SKIP`：正文三态可分辨、两类不串 `object_kind`、跨租户不可见、`limit` 生效且非正拒。
2. 端点实测（`:19081`，`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`）：合同页答一份 `SYN-CONTRACT-01/v1`，`contentRegistered:true`、规则包 `SYN-RULEPKG-01`、两条绑定恰好覆盖两种形状（`SYN-CHARGE-PREPAID`→指名策略、`SYN-CHARGE-COD`→显式不适用带依据）；协议页答两份新种子。
3. 页面层：WSL 起 vite（`:5199`，`PARCEL_API_TARGET` 指向上面那个实例），Edge 无头 `--dump-dom`。两页命中全部 `SYN` 值与中文状态词，**未命中**未配置码、「尚未接线」「访问通道尚未配置」与「0 份」。工作台总览随 `liveIds` 派生变档：全局已接线 11→13，主数据分区 7/13→9/13——这一格证的是页面没有自写第二份状态。
4. 全仓 `go test -count=1 ./...`：本笔涉及的包全绿。**唯一红的是 `internal/architecture` 的事务闭包门禁**，它网的是 `cmd/parcel-api/assemble_claims_test.go`（MCP-1 的 `bfabc0d`，不是本笔），已告知 MCP-1。

取证脚本留在本 feature 目录，是工具不是记录：`dom-dump.sh`（走 WSL 调 Windows Edge——PowerShell 5.1 会按控制台代码页重编码子进程 stdout，中文列头落盘即成问号，取证脚本自己把证据毁掉就没法核了）、`dom-check.sh`（该在的与不该在的两组一起核）、`workbench-count.sh`。下一页接线照用，改 needle 即可。
