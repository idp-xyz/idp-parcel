echo "== 上海→深圳快线周期安排（transport-fulfillment：让演示提交在一天里任何时刻都有一段运输机会可准备；票 demo-chain-gaps/18） =="
# 计划段窗口随提交时刻移动，按日运行的那一班只接得住提交时刻落在一小段里的单子；加快线是为了让首段窗口不论落在
# 哪一刻都含一班完整的出发与到达。只调演示租户的班期（合成值），不改匹配规则（ADR-0201 决定六）；取值见 README。
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/05-schedule-template-sha-szx-express-1200.json"
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/06-schedule-template-sha-szx-express-2000.json"
