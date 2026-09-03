# 来源连接器框架 + 首个连接器（CFETS 人民币中间价）；免复核声明格

Category: enhancement
Status: draft——出网方式与工件存放两处待定，见「先答再开工」
Blocked by: 03

## 要建什么

按 ADR-0099 决定六：

1. **连接器契约**（`internal/parcelpricing/adapters/sourcefeed/`）：`SourceConnector.Fetch(ctx, spec) (PublishedRecord, error)` → `PublishedRecord`（原文字节、内容 SHA-256、抓取时刻、地址、来源声明的公布日期）；`Transcribe(record, prior ReferenceSeriesRegistration | none) (ReferenceSeriesRegistrationSpec, error)` 产出**整版重述**的登记 spec：前一版全部期次 + 新期次，前一版开放末期在新期次起点闭合；凭证 = 工件引用（`sha256:… @ 抓取时刻 ← 地址`）。走同一登记用例。
2. **首个连接器：CFETS 人民币中间价**（官方、免费、稳定）。种类 `EXCHANGE_RATE`；口径引用来自租户对该来源的绑定配置（哪个商业价格政策版本声明「CFETS 中间价」这一牌价类型），**不在连接器里写死**。
3. **来源配置（实例半边的格）**：租户级「来源连接器绑定」——连接器种类、序列标识、口径引用、抓取节律、`免人工复核: 是/否/未声明`。未声明 = 需人工复核。免复核时由系统写复核记录，复核责任方为连接器身份、依据为该配置的版本。
4. **抓取失败的行为**：不补数、不沿用旧值、不登空版本；留缺口 + 告警（告警通道属实例半边，机制只出一条可观察记录，同 ADR-0095 两层）。
5. **进程口**：`cmd/parcel-pricing-feed`（受控批量口，与 `parcel-pricing-register` 同款：不监听端口，手跑或调度）。

## 先答再开工

- **出网**：parcel-api / worker 进程今天有没有出网能力与代理策略？本机 GitHub 都要走代理（workflow.md）；生产出网属部署实例半边。若无，本票只做契约 + 以本地文件为源的 `FileConnector`（供测试与受控导入），CFETS 连接器留 `draft`。
- **工件存放**：抓取原文放哪？ADR-0008 的 MinIO 证据存储是候选；ADR-0092 「本体存放是一条未配置的出向缝」是同形先例——摘要必备、定位符可缺。

## 红线

- 连接器不算数、不改数、不选口径。
- 不写任何真实来源的启用态；出厂零连接器绑定。
