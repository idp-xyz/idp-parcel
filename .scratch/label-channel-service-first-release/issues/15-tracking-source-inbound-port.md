# 15 轨迹源的拉取/接收端口不存在，形态也没选

Category: enhancement
Status: draft
Blocked by: 02, 03

## 缺口

[轨迹源盘点](../tracking-source-seam-inventory.md)第一段的结论是**整段无形状**，且零命中是
逐条搜过的：

- `17[Tt]rack|17TRACK|aftership|AfterShip|trackingmore|[Ww]ebhook` 扫全仓，命中全在 `docs/`
  与 `.scratch/`，`internal/`、`cmd/`、`migrations/`、`apps/` 一个都没有。
- 出向 HTTP 客户端全仓零匹配。
- 按出向接口命名扫 `internal/`，命中要么是防腐层朝内的 `*Source`，要么是发通知的
  `NotificationChannelGateway`（自称唯一实现是测试替身），要么是入向的 `accessidentity` 两口。
- inbox 机制承接的是内部上下文之间的事件：VE 侧十一个 `*_consumer.go` 逐个对应一个兄弟
  上下文，**无一来自进程外**。

## 做什么

定轨迹源的入站端口形状，**不接任何真实源**：

1. **形态选择**：轮询拉取、回调接收、还是两者都要。这一问必须答，因为两种形态的幂等与
   顺序保证完全不同——拉取要答「上次拉到哪」，接收要答「重复投递怎么办」。
2. 端口形状：一个源一个适配器，还是一个端口多家实现。17track 这类聚合平台一口给多家承运商
   的轨迹，而承运商直连是一家一口——**两者能不能共用一个端口，是本票的形状题**。
3. 出向调用的失败/超时/重试/幂等**照 `02` 的结论**，不另定一套。

## 为什么被 `03` 阻塞

端口交出来的东西要能被收编。`03` 不裁定收编方与时间认领口径，本票就只能猜一个交付形状，
而猜错的代价是端口白定。

## 红线

- 不填任何账号、密钥、轮询频率、状态码表（`PAR-INT-02`，实例半边）。
- **端口不得直接产出投影**：它交出的是待收编的原始素材，不是 `AcceptedSourceFact`
  （[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
  「不直插投影」，今天由 `domain.SourceContext` 封闭五值守着，本票不许松它）。

## 完成判据

形态选择有明确答复；端口形状落地并有替身测试；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[轨迹源盘点](../tracking-source-seam-inventory.md)第一段；票 `02`、`03`。
