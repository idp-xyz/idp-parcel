import { useEffect, useState } from 'react';
import {
  AlertTriangle,
  BadgeDollarSign,
  Boxes,
  ClipboardCheck,
  PackagePlus,
  RefreshCw,
  Route,
  Send,
  TabletSmartphone,
  Warehouse,
} from 'lucide-react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { listWorkDevices } from '../work-devices/api';
import {
  listOutboundRequests,
  listStockImports,
  listUnclaimedArrivals,
  listWarehouseCatalogue,
  listWarehouseChargeRequests,
  listWarehousePlans,
  listWaves,
} from './api';

const info = moduleInfoById['warehouse-dashboard'];

interface Metric {
  label: string;
  value: string;
  note: string;
  target: string;
  icon: typeof Warehouse;
}

function go(moduleId: string) {
  window.location.hash = `#/${moduleId}`;
}

/**
 * 仓配工作台：仓配系统首页。按岗位聚合实时待办计数、在用基础设施与四段履约动线入口。
 */
export function WarehouseDashboardPage() {
  const [loading, setLoading] = useState(true);
  const [counts, setCounts] = useState({
    pendingImports: '—',
    unclaimed: '—',
    reservedOutbound: '—',
    waves: '—',
    openTasks: '—',
    warehouses: '—',
    whDevices: '—',
    plans: '—',
    chargeRequests: '—',
  });

  const load = () => {
    setLoading(true);
    void Promise.all([
      listStockImports({ filters: { status: ['PENDING'] } }),
      listUnclaimedArrivals({ filters: { status: ['UNCLAIMED'] } }),
      listOutboundRequests({ filters: { status: ['RESERVED'] } }),
      listWaves({}),
      listWarehouseCatalogue({ selectors: { family: 'warehouse' }, filters: { deactivated: ['false'] } }),
      listWorkDevices({ filters: { status: ['REGISTERED'] } }),
      listWarehousePlans({}),
      listWarehouseChargeRequests({}),
    ]).then(([imp, unc, out, wav, wh, dev, pln, req]) => {
      const openTasks =
        wav.kind === 'outcome'
          ? wav.body.waves.reduce((sum, w) => sum + w.tasks.filter((t) => t.status === 'OPEN').length, 0)
          : null;
      const whDevices =
        dev.kind === 'outcome'
          ? dev.body.devices.filter((d) => d.workScopeKind === 'WAREHOUSE').length
          : null;
      setCounts({
        pendingImports: imp.kind === 'outcome' ? String(imp.body.page.total ?? imp.body.imports.length) : '—',
        unclaimed: unc.kind === 'outcome' ? String(unc.body.page.total ?? unc.body.arrivals.length) : '—',
        reservedOutbound: out.kind === 'outcome' ? String(out.body.page.total ?? out.body.requests.length) : '—',
        waves: wav.kind === 'outcome' ? String(wav.body.page.total ?? wav.body.waves.length) : '—',
        openTasks: openTasks === null ? '—' : String(openTasks),
        warehouses: wh.kind === 'outcome' ? String(wh.body.page.total ?? (wh.body.warehouses?.length ?? 0)) : '—',
        whDevices: whDevices === null ? '—' : String(whDevices),
        plans: pln.kind === 'outcome' ? String(pln.body.page.total ?? pln.body.plans.length) : '—',
        chargeRequests: req.kind === 'outcome' ? String(req.body.page.total ?? req.body.requests.length) : '—',
      });
      setLoading(false);
    });
  };

  useEffect(() => {
    load();
  }, []);

  const metrics: Metric[] = [
    { label: '待批建账导入', value: counts.pendingImports, note: '等待批准入账或驳回', target: 'stock-imports', icon: PackagePlus },
    { label: '待认领到货', value: counts.unclaimed, note: '无主到货与逾期到达', target: 'warehouse-unclaimed', icon: AlertTriangle },
    { label: '待放波次出库单', value: counts.reservedOutbound, note: '已预占，可合并释放波次', target: 'warehouse-outbound', icon: Send },
    { label: '待拣货任务', value: counts.openTasks, note: `共 ${counts.waves} 个已放波次`, target: 'warehouse-waves', icon: Route },
    { label: '在用仓库', value: counts.warehouses, note: '自营仓与代理仓', target: 'warehouse-catalogues', icon: Warehouse },
    { label: '在用仓库扫码设备', value: counts.whDevices, note: '实体终端与网页工位', target: 'warehouse-workstation', icon: TabletSmartphone },
    { label: '仓储计费方案', value: counts.plans, note: `累计 ${counts.chargeRequests} 笔计费请求`, target: 'warehouse-pricing', icon: BadgeDollarSign },
  ];

  const flowGroups = [
    {
      step: '01',
      title: '基础建档与规则',
      desc: '先建仓、库区、库位与包材，配置租户规则与扫码工位，再开始作业。',
      links: [
        { label: '仓库与库位', id: 'warehouse-catalogues' },
        { label: '货主与商品', id: 'warehouse-goods' },
        { label: '仓配规则设置', id: 'warehouse-settings' },
        { label: '扫描设备', id: 'work-devices' },
      ],
    },
    {
      step: '02',
      title: '入库与建账',
      desc: '受理入库预报或提交期初建账导入，在扫码作业台收货确认，处理待认领与退货。',
      links: [
        { label: '入库预报', id: 'warehouse-pre-advices' },
        { label: '建账导入', id: 'stock-imports' },
        { label: '待认领到货', id: 'warehouse-unclaimed' },
        { label: '退货管理', id: 'warehouse-returns' },
      ],
    },
    {
      step: '03',
      title: '实时库存与库内作业',
      desc: '查看商品格、整箱、占用与流水；执行上架移库、盘点审批、库存调整与加工换标。',
      links: [
        { label: '实时库存', id: 'warehouse-stock' },
        { label: '盘点与调整', id: 'warehouse-counts-adjustments' },
        { label: '加工与换标', id: 'warehouse-processing' },
      ],
    },
    {
      step: '04',
      title: '出库履约、扫码与计费',
      desc: '多选出库单合并放波次，在作业台按待办拣货复核打包发货，并跟进代理仓与仓储计费。',
      links: [
        { label: '出库订单', id: 'warehouse-outbound' },
        { label: '波次与拣货', id: 'warehouse-waves' },
        { label: '仓内作业台', id: 'warehouse-workstation' },
        { label: '代理仓履约', id: 'warehouse-agents' },
        { label: '仓储计费', id: 'warehouse-pricing' },
      ],
    },
  ];

  return (
    <div className="flex-1 overflow-y-auto bg-idpxyz-editor">
      <div className="max-w-[1160px] mx-auto px-6 py-6 space-y-6">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
            <p className="mt-1 type-body text-idpxyz-textMuted">
              以仓库作业流组织的运营总览：待办队列、现场扫码作业台与计费台账。未配置的租户规则仍如实答未配置。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" disabled={loading} onClick={load}>
              <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
              {loading ? '刷新中…' : '刷新数据'}
            </Button>
            <Button type="button" variant="outline" onClick={() => go('warehouse-workstation')}>
              <TabletSmartphone className="h-3.5 w-3.5 mr-1.5" />
              打开仓内作业台
            </Button>
          </div>
        </header>

        <section aria-label="关键待办与状态" className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
          {metrics.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.label}
                type="button"
                onClick={() => go(item.target)}
                className="text-left rounded-lg border border-idpxyz-border bg-idpxyz-panel p-4 hover:border-idpxyz-accent transition-colors"
              >
                <div className="flex items-center justify-between text-idpxyz-textMuted">
                  <span className="type-body">{item.label}</span>
                  <Icon className="h-4 w-4 shrink-0" />
                </div>
                <div className="mt-2 font-mono type-display font-bold text-idpxyz-textBright">{item.value}</div>
                <p className="mt-1 type-caption text-idpxyz-textMuted">{item.note}</p>
              </button>
            );
          })}
        </section>

        <section aria-label="仓配业务动线" className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {flowGroups.map((group) => (
            <Card key={group.step}>
              <CardHeader className="pb-2">
                <div className="flex items-center gap-2">
                  <span className="inline-flex h-6 w-6 items-center justify-center rounded bg-idpxyz-accent/15 font-mono type-caption font-bold text-idpxyz-accent">
                    {group.step}
                  </span>
                  <CardTitle className="type-section">{group.title}</CardTitle>
                </div>
                <p className="mt-1 type-body text-idpxyz-textMuted">{group.desc}</p>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-2">
                  {group.links.map((link) => (
                    <Button key={link.id} type="button" variant="outline" onClick={() => go(link.id)}>
                      {link.label}
                    </Button>
                  ))}
                </div>
              </CardContent>
            </Card>
          ))}
        </section>

        <section className="rounded-lg border border-idpxyz-border bg-idpxyz-panel p-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2.5">
            <Boxes className="h-5 w-5 text-idpxyz-accent shrink-0" />
            <div>
              <p className="type-body font-medium text-idpxyz-text">需要直接开始扫码收货、拣货或打包？</p>
              <p className="type-caption text-idpxyz-textMuted">
                在「扫描设备」为目标仓库登记一个网页工位设备并授予「作业事实登记」权限，即可无硬件操作全部现场作业。
              </p>
            </div>
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => go('warehouse-settings')}>
              <ClipboardCheck className="h-3.5 w-3.5 mr-1.5" />
              检查仓配规则设置
            </Button>
          </div>
        </section>
      </div>
    </div>
  );
}
