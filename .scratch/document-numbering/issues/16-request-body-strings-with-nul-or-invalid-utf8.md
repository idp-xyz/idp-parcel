# 16 写口请求体（JSON）里的字符串含 NUL 或非法 UTF-8，一律答 400、不落库、不悄悄改写

Category: bug
Status: in-progress——2026-11-02 作者认领，分支 `claude-docnum16`，从票 15 的 `b29cf8216` 起
父票：[spec](../spec.md)
Blocked by: 无（建在票 [15](./15-query-values-with-nul-or-invalid-utf8-across-http-reads.md) 之上：共用判据 `httpapi.StorableText` 与登记处挂检查的做法都出自那一票）
出处：
- 票 15 完成记录：「请求体（不在本票修）」一节，与「留给协调方」的「请求体 NUL 答 500，另立票」一条。
- 协调方 2026-11-02 裁定：NUL 与非法 UTF-8 两种都拒，答该族既有坏请求形状的 400，处理器之后的写一笔都不发生。非法 UTF-8 不替换成 U+FFFD：悄悄改写调用方送来的值，比拒收更糟。

## 现象

票 15 完成记录实测于 `969205ae9`，真库：

- 请求体 JSON 字符串里写 `\u0000`，解码后进命令的是真 NUL。
- `/commercial-business-party-registrations`、`/visibility-catalogue-milestone-mapping-registrations`、`/network-catalog-node-registrations` 三口都答 `500 NO_ANSWER_FORMED`：库拒收，调用方据 5xx 重发一个永远不会好的请求（ADR-0029 按恢复动作分格）。
- 载荷里的原始非法 UTF-8 字节，被 `encoding/json` 换成 U+FFFD 后照常受理。node 登记答 201，落库的值已被悄悄改过。

## 要做的

1. **清点。** 各族写口（POST / PUT / PATCH）在哪里读请求体：经共用解码件，还是各自解码。代表口探三种坏值今天答什么、落了什么：
   - JSON 字符串里的 `\u0000`；
   - 原始非法 UTF-8 字节；
   - JSON 转义的孤代理（如 `\ud800`，`encoding/json` 换成 U+FFFD）。
2. **修。** 能放一处就放一处。请求体出现下面任一种，就答该族既有「请求体读不出」那一格的 400，处理器之后的写一笔都不发生：
   - 任一字符串含 NUL；
   - 原始字节不是合法 UTF-8；
   - `\u` 转义出孤代理。

   非法 UTF-8 不替换。首方族同样拦，答首方族的坏请求形状（问题详情）。
3. **测试。**
   - 共用件单测：NUL 转义、原始非法字节、孤代理转义都拒；合法中文、emoji、代理对、`\\u0000`（转义的反斜杠）照收。
   - 每族一个代表写口的 HTTP 用例，断言 400 且不落库。
   - `cmd/parcel-api` 真库测试：票 15 探到的三个登记口修后答 400，库里无行。

## 不做

- 不改任何口的契约形状、成功答复与既有坏请求分格。
- 不加长度上限、字符白名单。
- 不管票 13 的账单号规整（那是领域规则，另有票）。

## 完成判据

- 清点表（口族、解码方式、修前、修后）写进完成记录。
- 受影响包、`internal/architecture` 与 `cmd/...` 真库测试过。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 照「进 main 的步骤」交非作者评审。
