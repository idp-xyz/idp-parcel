import { useEffect, useState } from 'react';
import { probeWarehouseSwitch } from './warehouse-switch-probe';
import { warehouseOffered, type WarehouseSwitch } from './warehouse-switch';

// 一页加载只问一次，不轮询：部署开关由后端进程在启动时读定，进程不重启它就不变。在途与答出的都记在模块级，缓存到页面生命周期
// 结束——Layout 重挂也只发一次请求，后挂载的第一帧就拿到答出的值。代价是后端重启、开关翻转之后，已开着的标签不会自己跟上
// （说明页上也没有「重新检查」），要手动重载页面才会再问；换来的是不轮询、不重试。
//
// 与授予自查（use-operator-grants.ts）不同，答不出也记：授予答不出多半是一时的，而这里答不出的后果只是仓配入口照旧摆
// ——与没有这个探测时一样——记住它就不会在后端起不来、网络不通时每挂一次就再发一次请求；要再问，同样手动重载页面。
let pending: Promise<WarehouseSwitch> | null = null;
let settled: WarehouseSwitch | undefined;

function loadWarehouseSwitch(): Promise<WarehouseSwitch> {
  if (pending === null) {
    pending = probeWarehouseSwitch().then((answer) => {
      settled = answer;
      return answer;
    });
  }
  return pending;
}

/**
 * 仓配入口摆不摆。答出之前、答不出、答出开着都是 true——入口照摆，让服务端去答；只有明确知道开关关着才变成 false。
 * 摆不摆只是呈现（口径同 useMayMakeDecision）：服务端照旧在每个仓配口上判开关。
 *
 * 状态存的就是这个布尔：开着或不知道时答复落下来不改状态，不多一次重渲。
 */
export function useWarehouseOffered(): boolean {
  const [offered, setOffered] = useState(() => warehouseOffered(settled));
  useEffect(() => {
    let cancelled = false;
    void loadWarehouseSwitch().then((answer) => {
      if (!cancelled) setOffered(warehouseOffered(answer));
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return offered;
}
