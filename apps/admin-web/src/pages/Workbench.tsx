// 落地页只陈述骨架现状，不放任何伪造的统计或示例业务数据——
// 红线要求未确认参数不得写成默认值，空态本身就是当前的真实状态。
export function Workbench() {
  return (
    <div className="flex-1 flex items-center justify-center">
      <div className="text-center max-w-[520px] px-6">
        <p className="text-[20px] mb-3 text-idpxyz-textBright">IDP Parcel 租户管理台</p>
        <p className="text-[13px] leading-6 text-idpxyz-textMuted">
          外壳骨架已就位：主题、导航与页面路由可用。左侧各治理模块尚未接线——
          业务 HTTP 端点按 ADR-0017 的准入闸门放行后，才会在这里出现真实数据。
        </p>
      </div>
    </div>
  );
}
