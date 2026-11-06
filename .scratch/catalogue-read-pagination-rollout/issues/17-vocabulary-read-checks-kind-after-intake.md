# 17 商业发布词表口先 Intake、后校验 kind

Category: bug
Status: ready-for-agent
Blocked by: 11（同一个文件的头注由 11 改过，11 进 main 之后再动）
地盘：`internal/partycommercial/adapters/http/publication_vocabulary.go` 及其用例。
出处：ADR-0055 决定三；11 的非作者评审（钉 `5b91b6517`）。

## 为什么

`GET /commercial-publication-vocabularies?kind=` 先校验 `kind` 再 Intake：渠道未配置时，集合外的 `kind` 答 400、合法的答 403，没有凭据的调用方
凭答复的不同就能探出类别的封闭集，与 ADR-0055 决定三「答复不随请求内容变」不符。迁移目录读口时，各票已把同形的选择器校验挪到 Intake 之后；这一口不是
目录读口，没有一并改。

## 做什么

- 先 Intake，再校验 `kind`：配置了的渠道上集合外或缺席照旧答 400、照旧带逐格问题；渠道未配置时与合规的一问逐字节同答 403。头注改成现状。
- 顺手查一遍其余上下文的非目录读口有没有同形的「先校验、后 Intake」；有就列进本票一并改，量大就另立票。

## 完成判据

- 端点用例：渠道未配置时，缺 `kind`、集合外 `kind`、合法 `kind` 三问逐字节同答 403；配置了的渠道上前两问照旧答 400 带逐格问题。
