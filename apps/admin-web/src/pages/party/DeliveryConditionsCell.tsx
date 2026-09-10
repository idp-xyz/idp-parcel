import type { DeliveryConditionRecord } from './api';

/**
 * 目录行上那一节交付条件的显法（票 admin-write-faces/25 裁读回折进两张既有目录行）：服务产品行显产品层，客户合同行显合同层
 * 连同所收紧的产品版本。没声明显「未声明」——那是这一版如实的状态，不显默认、不拿空数组冒充「没有方式」；页面先看键在不在
 * （与 contentRegistered 那条纪律同）。方式与规则引用是开放引用，照字面显、不译词。合同行只显自己这半（所收紧的是哪一版），
 * 不去产品行对照——这是选的不是漏的：两层一处对照归日后政策页一册的票。
 */
export function DeliveryConditionsCell({ conditions }: { conditions?: DeliveryConditionRecord }) {
  if (!conditions) {
    return <span className="text-idpxyz-textMuted">未声明</span>;
  }
  return (
    <div className="text-xs space-y-0.5">
      {conditions.tightens ? (
        <p className="text-idpxyz-textMuted">
          收紧自产品 <span className="font-mono text-idpxyz-text">{conditions.tightens.objectId}</span>
          <span className="font-mono text-idpxyz-text"> @ {conditions.tightens.version}</span>
        </p>
      ) : null}
      <p>
        <span className="text-idpxyz-textMuted">方式：</span>
        {conditions.methods.length === 0 ? (
          <span className="text-idpxyz-textMuted">（登记了零种——坏行，去查写侧）</span>
        ) : (
          conditions.methods.map((method, index) => (
            <span key={method} className="font-mono text-idpxyz-text">
              {index > 0 ? <span className="text-idpxyz-textMuted"> · </span> : null}
              {method}
            </span>
          ))
        )}
      </p>
      <p className="text-idpxyz-textMuted">
        收件范围 <span className="font-mono text-idpxyz-text">{conditions.recipientScopeRule}</span> · 交付证明{' '}
        <span className="font-mono text-idpxyz-text">{conditions.proofOfDeliveryRule}</span>
      </p>
    </div>
  );
}
