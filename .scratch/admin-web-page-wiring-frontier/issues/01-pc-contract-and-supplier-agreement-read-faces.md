# PC 客户合同与供应商协议查阅面——同表同读法，两页一并接线

Category: enhancement
Status: in-progress

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
