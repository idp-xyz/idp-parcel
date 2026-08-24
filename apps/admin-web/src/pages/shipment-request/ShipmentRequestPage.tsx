import { useState } from 'react';
import { SubmitShipmentRequestPage } from './SubmitShipmentRequestPage';
import { WithdrawShipmentRequestPage } from './WithdrawShipmentRequestPage';

// 单导航位入口:提交与撤回是同一份委托生命周期上相邻的两个客户动作,共用一个
// 页面位并在页内切换。导航接线归装配侧;若那边更愿意给两个导航位,直接改用
// 本目录分别导出的两个页面组件即可,本组件不是必经之路。
export function ShipmentRequestPage() {
  const [view, setView] = useState<'submit' | 'withdraw'>('submit');

  const tabClass = (active: boolean) =>
    `px-3 py-1.5 text-[13px] rounded border ${
      active
        ? 'border-idpxyz-accent text-idpxyz-accent'
        : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
    }`;

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="shrink-0 flex gap-2 px-6 pt-4">
        <button type="button" className={tabClass(view === 'submit')} onClick={() => setView('submit')}>
          提交服务请求
        </button>
        <button
          type="button"
          className={tabClass(view === 'withdraw')}
          onClick={() => setView('withdraw')}
        >
          决定前撤回
        </button>
      </div>
      {view === 'submit' ? <SubmitShipmentRequestPage /> : <WithdrawShipmentRequestPage />}
    </div>
  );
}
