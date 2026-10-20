echo "== 容量池登记（transport-fulfillment：按日期物化登记干线一班，随周期安排的逐维容量建池；票 demo-chain-gaps/17） =="
# 上面三份周期安排各声明了重量与件数两维的硬上限（合成值，不是任何车辆或舱位的真实载量），按日期物化登记的班次
# 随之各建两池（ADR-0201 决定三）。这里只显式登干线一班，让查阅口 capacity-pool 有池可列；准备评估的候选照旧按
# 日期现算，不靠这一班。运行日期是固定的合成日期，不是「今天」：池与班次的身份都由它派生，重灌同一份答已存在。
"$BIN/parcel-transport-register" schedule -input "$SEEDS/transport/04-materialized-schedule-szx-sin.json"
