import { useMemo, useState } from 'react';
import {
  BadgeDollarSign,
  Building2,
  ClipboardList,
  Scale,
  Settings,
  ShieldCheck,
  Sliders,
  TabletSmartphone,
} from 'lucide-react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import { useCatalogueCursor } from '../use-catalogue-cursor';
import { listDisposalRules } from './api';
import {
  internalOpsActionSpecs,
  outboundWaveActionSpecs,
  pricingActionSpecs,
  receivingActionSpecs,
} from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-settings'];

interface RuleCardSpec {
  actionId: string;
  group: 'stock' | 'charge' | 'agent';
  title: string;
  desc: string;
  icon: typeof Settings;
}

const ruleCards: readonly RuleCardSpec[] = [
  {
    actionId: 'allocation-strategy',
    group: 'stock',
    title: '采用参考分配策略',
    desc: '选择仓配分配策略（如先过期先出/先进先出/优先拣货位），未采用前出库分配按租户未配置处理。',
    icon: Sliders,
  },
  {
    actionId: 'shortage-policy',
    group: 'stock',
    title: '货主缺货处置规则',
    desc: '决定货主出库单库存不足时是整单拒绝（REJECT）还是按可用量部分预占（PARTIAL）。',
    icon: ClipboardList,
  },
  {
    actionId: 'approval-duty',
    group: 'stock',
    title: '库存调整批准职责规则',
    desc: '配置建账导入、库存调整、盘点差异与作业差异是否要求提交人与批准人分离。',
    icon: Scale,
  },
  {
    actionId: 'disposal-rule',
    group: 'stock',
    title: '待认领到货处置规则',
    desc: '按仓库登记无主到货超期后的允许处置方式（销毁、捐出、退回等）与认领期限。',
    icon: ShieldCheck,
  },
  {
    actionId: 'charge-units',
    group: 'charge',
    title: '仓的重量与体积单位',
    desc: '声明各仓库出库与仓储收费计量所采用的重量单位与体积单位。',
    icon: BadgeDollarSign,
  },
  {
    actionId: 'charge-aging',
    group: 'charge',
    title: '仓租库龄口径',
    desc: '按仓库设置仓租库龄口径（按实批或按理论先进先出）。',
    icon: BadgeDollarSign,
  },
  {
    actionId: 'charge-project',
    group: 'charge',
    title: '增值服务项目',
    desc: '登记贴标、质检、拍照、换箱等仓内增值服务项目名称，供作业与计费引用。',
    icon: BadgeDollarSign,
  },
  {
    actionId: 'agent-connector',
    group: 'agent',
    title: '代理仓连接器配置',
    desc: '为类型为「代理仓」的仓库开启外部履约连接器，使其支持指令转出与结果回传。',
    icon: Building2,
  },
];

/** 仓配规则设置中心：把原先散落在 4 个业务页里的 8 项租户级规则集中收口，并内联展示已登记的待认领处置规则名册。 */
export function WarehouseSettingsPage() {
  const lookups = useWarehouseLookups();
  const allSpecs = useMemo(
    () => [...receivingActionSpecs, ...internalOpsActionSpecs, ...outboundWaveActionSpecs, ...pricingActionSpecs],
    [],
  );
  const targetMap = useWarehouseTargetMap(allSpecs);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const { answer: rulesAnswer, retry: reloadRules } = useCatalogueCursor({}, listDisposalRules);
  const disposalRules = rulesAnswer?.kind === 'outcome' ? rulesAnswer.body.disposalRules : [];

  const openRule = (actionId: string) => {
    const target = targetMap.get(actionId);
    if (!target) return;
    setDrawer((current) => ({ target, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const renderGroup = (group: RuleCardSpec['group'], heading: string, note: string) => {
    const items = ruleCards.filter((c) => c.group === group);
    return (
      <section className="space-y-3">
        <div>
          <h2 className="type-section font-semibold text-idpxyz-textBright">{heading}</h2>
          <p className="type-caption text-idpxyz-textMuted">{note}</p>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {items.map((item) => {
            const Icon = item.icon;
            const target = targetMap.get(item.actionId);
            return (
              <Card key={item.actionId}>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Icon className="h-4 w-4 shrink-0" />
                    <CardTitle className="type-section">{item.title}</CardTitle>
                  </div>
                  <p className="mt-1 type-body text-idpxyz-textMuted">{item.desc}</p>
                </CardHeader>
                <CardContent>
                  {target ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openRule(item.actionId)}>
                      配置 / 登记新修订
                    </Button>
                  ) : (
                    <p className="type-caption text-idpxyz-textMuted">当前操作者未持有此规则的登记授予。</p>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      </section>
    );
  };

  return (
    <div className="flex-1 overflow-y-auto bg-idpxyz-editor">
      <div className="max-w-[1080px] mx-auto px-6 py-6 space-y-8">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
            <p className="mt-1 type-body text-idpxyz-textMuted">
              集中管理全部租户级仓配规则，业务作业页不再混入参数设置。凡租户未配置的项，系统运行期均如实答未配置，不套用硬编码默认值。
            </p>
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/work-devices'; }}>
              <TabletSmartphone className="h-3.5 w-3.5 mr-1.5" />
              管理扫描设备
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/operator-access'; }}>
              管理操作者与授予
            </Button>
          </div>
        </header>

        {renderGroup('stock', '一、库存分配、审批与异常处置规则', '影响出库预占、建账与调账审批、以及无主到货处置。')}
        {renderGroup('charge', '二、仓储计费口径与增值服务', '影响操作费计量、库龄仓租阶梯与增值服务发生项。')}
        {renderGroup('agent', '三、外部代理仓对接', '为代理仓绑定外部履约连接器。')}

        <section className="space-y-3">
          <h2 className="type-section font-semibold text-idpxyz-textBright">已登记的待认领处置规则</h2>
          <div className="rounded-lg border border-idpxyz-border bg-idpxyz-panel overflow-x-auto">
            {disposalRules.length === 0 ? (
              <p className="p-4 type-body text-idpxyz-textMuted">暂未登记任何仓库的待认领处置规则（按未配置处理）。</p>
            ) : (
              <table className="w-full border-collapse">
                <thead>
                  <tr className="bg-idpxyz-editor/60">
                    <th className="px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border">仓库</th>
                    <th className="px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border">修订</th>
                    <th className="px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border">保管天数</th>
                    <th className="px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border">允许处置方式</th>
                    <th className="px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border">登记人 / 时刻</th>
                  </tr>
                </thead>
                <tbody>
                  {disposalRules.map((rule) => (
                    <tr key={`${rule.warehouseCode}-${rule.revision}`} className="border-b border-idpxyz-border/60">
                      <td className="px-3 py-2 font-mono type-body">{rule.warehouseCode}</td>
                      <td className="px-3 py-2 font-mono type-body">{`r${rule.revision}`}</td>
                      <td className="px-3 py-2 font-mono type-body">{`${Math.round(rule.claimWindowSeconds / 86400)} 天（${rule.claimWindowSeconds}s）`}</td>
                      <td className="px-3 py-2 type-body">{rule.disposalMethods.join('、')}</td>
                      <td className="px-3 py-2 font-mono type-caption text-idpxyz-textMuted">
                        {`${rule.registrant} · ${formatInstant(rule.registeredAt)}`}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </section>
      </div>

      <ActionDrawer
        moduleId="warehouse-settings"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => {
          reloadRules();
          lookups.reload();
        }}
      />
    </div>
  );
}
