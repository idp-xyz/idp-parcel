import { useCallback, useEffect, useMemo, useState } from 'react';
import { Maximize2, RefreshCw, ScanBarcode, TabletSmartphone } from 'lucide-react';
import { Button, Input, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import { postWorkFact } from '../catalogue-api';
import { listWorkDevices, type WorkDeviceRecord } from '../work-devices/api';
import { asWarehouseAction, type WarehouseActionBody } from './actions';
import {
  listOutboundRequests,
  listOwnerPreAdvices,
  listWaves,
  type OutboundRequestRecord,
  type PreAdviceRecord,
  type WaveTaskRecord,
} from './api';
import { GuidedFormBody, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormSeed, GuidedFormValues } from './guided-forms';
import {
  advanceDeviceSequence,
  browserStorage,
  readActiveDeviceId,
  readDeviceSequence,
  registeredDeviceConditions,
  writeActiveDeviceId,
} from './station/device-binding';
import { dispatchStationHash, packStationHash, pickStationHash, putawayStationHash, receiveStationHash } from './station/stations';
import { useWarehouseLookups } from './use-catalogues';
import {
  workFactOutcomeLabels,
  workFactPrefillFor,
  workFactProblemNote,
  workFactSpecs,
  type WorkFactSpec,
} from './work-fact';

const info = moduleInfoById['warehouse-workstation'];

type StationTab = 'inbound' | 'picking' | 'pack' | 'dispatch' | 'internal';

interface OpenPickItem {
  waveId: string;
  task: WaveTaskRecord;
}

const inboundIds = ['receipt-goods', 'receipt-carton', 'warehouse-measurement', 'unclaimed-arrival', 'return-receipt'] as const;
const packIds = ['outbound-check', 'pack'] as const;
const dispatchIds = ['outbound-confirmation', 'pickup-handover', 'stock-return'] as const;
const internalIds = [
  'internal-move',
  'form-conversion',
  'condition-conversion',
  'shortage-registration',
  'processing-execution',
  'relabel-execution',
  'unboxing',
  'count-entry',
] as const;

function specById(id: string): WorkFactSpec {
  return workFactSpecs.find((s) => s.id === id) ?? workFactSpecs[0];
}

/**
 * 仓内扫码作业台（任务队列驱动）：
 * 顶部锁定当前仓库设备与扫码输入；五个工位签（收货台 / 拣货车 / 复核打包台 / 出库发货台 / 库内与盘点）
 * 各自展示本仓真实待办队列，点击待办任务或扫码即自动带出单号与明细，提交后自动刷新待办队列并递增设备序号。
 */
export function WarehouseWorkstationPage() {
  const lookups = useWarehouseLookups();

  const [devices, setDevices] = useState<WorkDeviceRecord[]>([]);
  const [loadingDevices, setLoadingDevices] = useState(true);
  const [deviceError, setDeviceError] = useState<string | null>(null);
  const [activeDeviceId, setActiveDeviceId] = useState<string>(() => readActiveDeviceId(browserStorage()));
  const [seqDisplay, setSeqDisplay] = useState<number>(() =>
    readDeviceSequence(browserStorage(), readActiveDeviceId(browserStorage())),
  );

  const [station, setStation] = useState<StationTab>('picking');
  const [activeSpecId, setActiveSpecId] = useState<string>('pick');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);
  const [scanText, setScanText] = useState('');
  const [scanHint, setScanHint] = useState<string | null>(null);

  const [ownerFilter, setOwnerFilter] = useState('');
  const [preAdvices, setPreAdvices] = useState<PreAdviceRecord[]>([]);
  const [openPicks, setOpenPicks] = useState<OpenPickItem[]>([]);
  const [pickedOrders, setPickedOrders] = useState<OutboundRequestRecord[]>([]);
  const [checkedOrders, setCheckedOrders] = useState<OutboundRequestRecord[]>([]);
  const [packedOrders, setPackedOrders] = useState<OutboundRequestRecord[]>([]);
  const [loadingQueues, setLoadingQueues] = useState(false);

  const loadDevices = useCallback(() => {
    setLoadingDevices(true);
    setDeviceError(null);
    void listWorkDevices(registeredDeviceConditions).then((res) => {
      setLoadingDevices(false);
      if (res.kind !== 'outcome') {
        setDeviceError(
          res.kind === 'callerProblem'
            ? res.detail || res.code
            : res.kind === 'transport'
              ? res.message
              : '无法读取工作设备名册',
        );
        return;
      }
      const warehouseDevices = res.body.devices.filter((d) => d.workScopeKind === 'WAREHOUSE');
      setDevices(warehouseDevices);
      if (warehouseDevices.length > 0) {
        const stored = readActiveDeviceId(browserStorage());
        const chosen = warehouseDevices.some((d) => d.deviceId === stored) ? stored : warehouseDevices[0].deviceId;
        setActiveDeviceId(chosen);
        writeActiveDeviceId(browserStorage(), chosen);
        setSeqDisplay(readDeviceSequence(browserStorage(), chosen));
      }
    });
  }, []);

  useEffect(() => {
    loadDevices();
  }, [loadDevices]);

  useEffect(() => {
    if (ownerFilter === '' && lookups.owners.length > 0) {
      setOwnerFilter(lookups.owners[0].accountId);
    }
  }, [lookups.owners, ownerFilter]);

  const activeDevice = useMemo(
    () => devices.find((d) => d.deviceId === activeDeviceId) ?? null,
    [devices, activeDeviceId],
  );
  const activeWarehouse = activeDevice?.workScopeRef ?? '';

  const loadQueues = useCallback(() => {
    if (!activeWarehouse) return;
    setLoadingQueues(true);
    const promises: Array<Promise<void>> = [
      listWaves({}).then((res) => {
        if (res.kind !== 'outcome') return;
        const items: OpenPickItem[] = [];
        for (const w of res.body.waves) {
          if (w.warehouseCode !== activeWarehouse) continue;
          for (const task of w.tasks) {
            if (task.status === 'OPEN') items.push({ waveId: w.waveId, task });
          }
        }
        setOpenPicks(items);
      }),
      listOutboundRequests({ filters: { status: ['PICKED'] } }).then((res) => {
        if (res.kind === 'outcome') {
          setPickedOrders(res.body.requests.filter((r) => r.warehouseCode === activeWarehouse));
        }
      }),
      listOutboundRequests({ filters: { status: ['CHECKED'] } }).then((res) => {
        if (res.kind === 'outcome') {
          setCheckedOrders(res.body.requests.filter((r) => r.warehouseCode === activeWarehouse));
        }
      }),
      listOutboundRequests({ filters: { status: ['PACKED'] } }).then((res) => {
        if (res.kind === 'outcome') {
          setPackedOrders(res.body.requests.filter((r) => r.warehouseCode === activeWarehouse));
        }
      }),
    ];
    if (ownerFilter.trim()) {
      promises.push(
        listOwnerPreAdvices(ownerFilter.trim(), '').then((res) => {
          if (res.kind === 'outcome') {
            setPreAdvices(
              res.body.preAdvices.filter(
                (p) => p.warehouseCode === activeWarehouse && (p.status === 'ADVISED' || p.status === 'RECEIVING'),
              ),
            );
          }
        }),
      );
    }
    void Promise.all(promises).finally(() => setLoadingQueues(false));
  }, [activeWarehouse, ownerFilter]);

  useEffect(() => {
    loadQueues();
  }, [loadQueues]);

  const prefillForm = useCallback(
    (specId: string, extra: GuidedFormValues) => {
      const spec = specById(specId);
      const base = activeDevice
        ? workFactPrefillFor(
            spec,
            activeDevice.deviceId,
            activeDevice.workScopeRef,
            readDeviceSequence(browserStorage(), activeDevice.deviceId) + 1,
          )
        : {};
      setActiveSpecId(specId);
      setSeed((current) => ({ targetId: specId, values: { ...base, ...extra }, nonce: (current?.nonce ?? 0) + 1 }));
    },
    [activeDevice],
  );

  // 设备就绪或工位切换时自动给当前动作补齐设备签发头（factId / warehouse / occurredAt / deviceSequence）
  useEffect(() => {
    if (!activeDevice) return;
    if (seed?.targetId === activeSpecId) return;
    prefillForm(activeSpecId, {});
  }, [activeDevice, activeSpecId, seed?.targetId, prefillForm]);

  const switchStation = (next: StationTab) => {
    setStation(next);
    const defaultSpec: Record<StationTab, string> = {
      inbound: 'receipt-goods',
      picking: 'pick',
      pack: 'outbound-check',
      dispatch: 'outbound-confirmation',
      internal: 'internal-move',
    };
    prefillForm(defaultSpec[next], {});
  };

  const handleSelectDevice = (deviceId: string) => {
    setActiveDeviceId(deviceId);
    writeActiveDeviceId(browserStorage(), deviceId);
    setSeqDisplay(readDeviceSequence(browserStorage(), deviceId));
    setSeed(null);
  };

  const handleScanSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const code = scanText.trim();
    if (!code) return;
    setScanText('');
    const matchedPick = openPicks.find((p) => p.task.taskId === code || p.task.requestId === code);
    if (matchedPick) {
      setStation('picking');
      prefillForm('pick', {
        taskId: matchedPick.task.taskId,
        quantity: String(matchedPick.task.quantity),
      });
      setScanHint(`已匹配待拣任务 ${matchedPick.task.taskId}`);
      return;
    }
    const matchedPicked = pickedOrders.find((r) => r.requestId === code || r.documentNumber === code);
    if (matchedPicked) {
      setStation('pack');
      prefillForm('outbound-check', { requestId: matchedPicked.requestId });
      setScanHint(`已匹配待复核出库单 ${matchedPicked.requestId}`);
      return;
    }
    const matchedChecked = checkedOrders.find((r) => r.requestId === code || r.documentNumber === code);
    if (matchedChecked) {
      setStation('pack');
      prefillForm('pack', { requestId: matchedChecked.requestId });
      setScanHint(`已匹配待打包出库单 ${matchedChecked.requestId}`);
      return;
    }
    const matchedPacked = packedOrders.find((r) => r.requestId === code || r.documentNumber === code);
    if (matchedPacked) {
      setStation('dispatch');
      prefillForm(matchedPacked.dispatch === 'PICKUP' ? 'pickup-handover' : 'outbound-confirmation', {
        requestId: matchedPacked.requestId,
        parcelSequence: '1',
      });
      setScanHint(`已匹配待发货出库单 ${matchedPacked.requestId}`);
      return;
    }
    const matchedAdvice = preAdvices.find((p) => p.preAdviceId === code || p.documentNumber === code);
    if (matchedAdvice) {
      setStation('inbound');
      prefillForm(matchedAdvice.inboundMode === 'BY_CARTON' ? 'receipt-carton' : 'receipt-goods', {
        preAdviceId: matchedAdvice.preAdviceId,
      });
      setScanHint(`已匹配入库预报 ${matchedAdvice.preAdviceId}`);
      return;
    }
    setScanHint(`当前本仓待办队列未匹配到「${code}」，请直接在下方作业表单录入。`);
  };

  const activeSpec = specById(activeSpecId);

  const activeTarget = useMemo<GuidedActionTarget | null>(() => {
    if (!activeDevice) return null;
    const headers = {
      deviceId: activeDevice.deviceId,
      scopeKind: 'WAREHOUSE',
      scopeReference: activeDevice.workScopeRef,
    };
    return {
      id: activeSpec.id,
      label: activeSpec.label,
      title: activeSpec.title,
      endpoint: `POST ${activeSpec.endpoint}`,
      snapshotHint: activeSpec.snapshotHint,
      outcomeLabels: workFactOutcomeLabels,
      form: activeSpec.form,
      submit: async (snapshot: unknown) => {
        const body =
          snapshot && typeof snapshot === 'object'
            ? { ...(activeSpec.fixedPayload ?? {}), ...(snapshot as Record<string, unknown>) }
            : snapshot;
        let usedSeq: number | undefined;
        if (activeSpec.usesDeviceSequence && body && typeof body === 'object') {
          const rec = body as Record<string, unknown>;
          if (typeof rec.deviceSequence === 'number' && Number.isFinite(rec.deviceSequence)) {
            usedSeq = rec.deviceSequence;
          } else {
            usedSeq = readDeviceSequence(browserStorage(), activeDevice.deviceId) + 1;
            rec.deviceSequence = usedSeq;
          }
        }
        const answer = await postWorkFact<WarehouseActionBody>(activeSpec.endpoint, body, headers);
        if (answer.kind !== 'outcome') return answer;
        if (activeSpec.usesDeviceSequence && usedSeq !== undefined) {
          setSeqDisplay(advanceDeviceSequence(browserStorage(), activeDevice.deviceId, usedSeq));
        }
        return { kind: 'outcome', status: answer.status, body: asWarehouseAction(answer.body) };
      },
    };
  }, [activeDevice, activeSpec]);

  const tabButtonClass = (active: boolean) =>
    `px-2.5 py-1 rounded border type-caption transition-colors ${
      active
        ? 'bg-idpxyz-accent text-idpxyz-buttonFg border-idpxyz-accent font-medium'
        : 'bg-idpxyz-panel text-idpxyz-text border-idpxyz-border hover:bg-idpxyz-hover'
    }`;

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <div className="px-6 pt-4 pb-3 border-b border-idpxyz-border bg-idpxyz-panel space-y-3 shrink-0">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2">
              <TabletSmartphone className="h-5 w-5 text-idpxyz-accent" />
              <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
            </div>
            {devices.length > 0 ? (
              <div className="flex items-center gap-2">
                <select
                  aria-label="当前工作设备"
                  className="h-8 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2.5 font-mono type-body text-idpxyz-text"
                  value={activeDeviceId}
                  onChange={(e) => handleSelectDevice(e.target.value)}
                >
                  {devices.map((d) => (
                    <option key={d.deviceId} value={d.deviceId}>
                      {`${d.deviceId} · 仓 ${d.workScopeRef}`}
                    </option>
                  ))}
                </select>
                <span className="font-mono type-caption text-idpxyz-textMuted">{`序号 #${seqDisplay}`}</span>
              </div>
            ) : null}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <form onSubmit={handleScanSubmit} className="flex items-center gap-1.5">
              <ScanBarcode className="h-4 w-4 text-idpxyz-textMuted" />
              <Input
                className="h-8 w-56 font-mono type-body"
                value={scanText}
                onChange={(e) => setScanText(e.target.value)}
                placeholder="扫单号/任务号回车自动定位"
              />
            </form>
            <Button type="button" variant="outline" size="sm" onClick={() => enterStation(pickStationHash())}>
              <Maximize2 className="h-3.5 w-3.5 mr-1" />
              全屏拣货
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => enterStation(packStationHash())}>
              <Maximize2 className="h-3.5 w-3.5 mr-1" />
              全屏复核打包
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => enterStation(receiveStationHash())}>
              <Maximize2 className="h-3.5 w-3.5 mr-1" />
              全屏收货
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => enterStation(dispatchStationHash())}>
              <Maximize2 className="h-3.5 w-3.5 mr-1" />
              全屏发货
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => enterStation(putawayStationHash())}>
              <Maximize2 className="h-3.5 w-3.5 mr-1" />
              全屏移库
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={loadingQueues} onClick={loadQueues}>
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新待办
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                window.location.hash = '#/work-devices';
              }}
            >
              设备管理
            </Button>
          </div>
        </div>

        {scanHint ? <p className="type-caption text-idpxyz-accent">{scanHint}</p> : null}

        <Tabs value={station} onValueChange={(next) => switchStation(next as StationTab)}>
          <TabsList>
            <TabsTrigger value="inbound">{`收货台 (${preAdvices.length})`}</TabsTrigger>
            <TabsTrigger value="picking">{`拣货车 (${openPicks.length})`}</TabsTrigger>
            <TabsTrigger value="pack">{`复核打包台 (${pickedOrders.length + checkedOrders.length})`}</TabsTrigger>
            <TabsTrigger value="dispatch">{`出库发货台 (${packedOrders.length})`}</TabsTrigger>
            <TabsTrigger value="internal">库内与盘点</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {loadingDevices ? (
        <div className="p-6 type-body text-idpxyz-textMuted">正在加载在用仓库扫描设备…</div>
      ) : deviceError ? (
        <div className="p-6 type-body text-idpxyz-danger">{deviceError}</div>
      ) : !activeDevice ? (
        <div className="p-6">
          <div className="max-w-xl rounded-lg border border-idpxyz-border bg-idpxyz-panel p-5 space-y-3">
            <h2 className="type-section font-semibold text-idpxyz-textBright">还没有在用的仓库工作设备</h2>
            <p className="type-body text-idpxyz-textMuted">
              仓内作业需要绑定一台归属具体仓库（WAREHOUSE）的在用设备。去「扫描设备」登记一台网页工位设备，并确认操作者已持有该仓「作业事实登记」授予后即可开工。
            </p>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/work-devices'; }}>
              去扫描设备登记
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex-1 flex overflow-hidden">
          <aside className="w-80 shrink-0 border-r border-idpxyz-border bg-idpxyz-panel overflow-y-auto p-4 space-y-3">
            {station === 'inbound' ? (
              <>
                <div className="space-y-1.5">
                  <div className="type-caption font-semibold text-idpxyz-textBright">本仓待收货预报</div>
                  <select
                    className="h-8 w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-caption text-idpxyz-text"
                    value={ownerFilter}
                    onChange={(e) => setOwnerFilter(e.target.value)}
                  >
                    <option value="">选择货主筛选预报…</option>
                    {lookups.owners.map((o) => (
                      <option key={o.accountId} value={o.accountId}>
                        {o.customerPartyName ? `${o.accountId} · ${o.customerPartyName}` : o.accountId}
                      </option>
                    ))}
                  </select>
                </div>
                {preAdvices.length === 0 ? (
                  <p className="type-caption text-idpxyz-textMuted">当前仓库/货主暂无待收货入库预报。</p>
                ) : (
                  <div className="space-y-2">
                    {preAdvices.map((p) => (
                      <button
                        key={p.preAdviceId}
                        type="button"
                        onClick={() =>
                          prefillForm(p.inboundMode === 'BY_CARTON' ? 'receipt-carton' : 'receipt-goods', {
                            preAdviceId: p.preAdviceId,
                          })
                        }
                        className="w-full text-left rounded border border-idpxyz-border bg-idpxyz-editor p-2.5 hover:border-idpxyz-accent transition-colors"
                      >
                        <div className="font-mono type-body font-semibold text-idpxyz-textBright">
                          {p.documentNumber ?? p.preAdviceId}
                        </div>
                        <div className="mt-0.5 type-caption text-idpxyz-textMuted">
                          {`${p.inboundMode === 'BY_CARTON' ? '整箱' : '商品'} · ${p.status === 'RECEIVING' ? '收货中' : '已预报'}`}
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </>
            ) : null}

            {station === 'picking' ? (
              <>
                <div className="type-caption font-semibold text-idpxyz-textBright">
                  {`本仓待拣货任务（${openPicks.length}）`}
                </div>
                {openPicks.length === 0 ? (
                  <p className="type-caption text-idpxyz-textMuted">本仓暂无待拣任务。请先在「出库单」释放波次。</p>
                ) : (
                  <div className="space-y-2">
                    {openPicks.map(({ waveId, task }) => (
                      <button
                        key={task.taskId}
                        type="button"
                        onClick={() =>
                          prefillForm('pick', {
                            taskId: task.taskId,
                            quantity: String(task.quantity),
                          })
                        }
                        className="w-full text-left rounded border border-idpxyz-border bg-idpxyz-editor p-2.5 hover:border-idpxyz-accent transition-colors"
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono type-body font-semibold text-idpxyz-textBright">{task.locationCode}</span>
                          <span className="font-mono type-caption text-idpxyz-accent">{`×${task.quantity} ${task.unit}`}</span>
                        </div>
                        <div className="mt-0.5 font-mono type-caption text-idpxyz-text">{task.goodsCode}</div>
                        <div className="mt-0.5 font-mono type-caption text-idpxyz-textMuted">
                          {`${task.taskId} · 波次 ${waveId}`}
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </>
            ) : null}

            {station === 'pack' ? (
              <div className="space-y-4">
                <div className="space-y-2">
                  <div className="type-caption font-semibold text-idpxyz-textBright">{`待复核（${pickedOrders.length}）`}</div>
                  {pickedOrders.length === 0 ? (
                    <p className="type-caption text-idpxyz-textMuted">暂无待复核出库单。</p>
                  ) : (
                    pickedOrders.map((r) => (
                      <button
                        key={r.requestId}
                        type="button"
                        onClick={() => prefillForm('outbound-check', { requestId: r.requestId })}
                        className="w-full text-left rounded border border-idpxyz-border bg-idpxyz-editor p-2.5 hover:border-idpxyz-accent transition-colors"
                      >
                        <div className="font-mono type-body font-semibold text-idpxyz-textBright">
                          {r.documentNumber ?? r.requestId}
                        </div>
                        <div className="mt-0.5 type-caption text-idpxyz-textMuted">
                          {`${r.ownerAccount} · ${r.recipient} · 需复核 ${r.wantedQuantity}`}
                        </div>
                      </button>
                    ))
                  )}
                </div>

                <div className="space-y-2">
                  <div className="type-caption font-semibold text-idpxyz-textBright">{`待打包（${checkedOrders.length}）`}</div>
                  {checkedOrders.length === 0 ? (
                    <p className="type-caption text-idpxyz-textMuted">暂无待打包出库单。</p>
                  ) : (
                    checkedOrders.map((r) => (
                      <button
                        key={r.requestId}
                        type="button"
                        onClick={() => prefillForm('pack', { requestId: r.requestId })}
                        className="w-full text-left rounded border border-idpxyz-border bg-idpxyz-editor p-2.5 hover:border-idpxyz-accent transition-colors"
                      >
                        <div className="font-mono type-body font-semibold text-idpxyz-textBright">
                          {r.documentNumber ?? r.requestId}
                        </div>
                        <div className="mt-0.5 type-caption text-idpxyz-textMuted">
                          {`${r.ownerAccount} · ${r.recipient} · 待选包材`}
                        </div>
                      </button>
                    ))
                  )}
                </div>
              </div>
            ) : null}

            {station === 'dispatch' ? (
              <div className="space-y-2">
                <div className="type-caption font-semibold text-idpxyz-textBright">{`待出库发货（${packedOrders.length}）`}</div>
                {packedOrders.length === 0 ? (
                  <p className="type-caption text-idpxyz-textMuted">本仓暂无已打包待出库订单。</p>
                ) : (
                  packedOrders.map((r) => (
                    <button
                      key={r.requestId}
                      type="button"
                      onClick={() =>
                        prefillForm(r.dispatch === 'PICKUP' ? 'pickup-handover' : 'outbound-confirmation', {
                          requestId: r.requestId,
                          parcelSequence: '1',
                        })
                      }
                      className="w-full text-left rounded border border-idpxyz-border bg-idpxyz-editor p-2.5 hover:border-idpxyz-accent transition-colors"
                    >
                      <div className="font-mono type-body font-semibold text-idpxyz-textBright">
                        {r.documentNumber ?? r.requestId}
                      </div>
                      <div className="mt-0.5 type-caption text-idpxyz-textMuted">
                        {`${r.dispatch === 'PICKUP' ? '自提交付' : '出库确认'} · ${r.recipient}`}
                      </div>
                    </button>
                  ))
                )}
              </div>
            ) : null}

            {station === 'internal' ? (
              <div className="space-y-1.5">
                <div className="type-caption font-semibold text-idpxyz-textBright">库内作业类型</div>
                {internalIds.map((id) => {
                  const s = specById(id);
                  return (
                    <button
                      key={id}
                      type="button"
                      onClick={() => prefillForm(id, {})}
                      className={`w-full text-left rounded border px-3 py-2 type-body transition-colors ${
                        activeSpecId === id
                          ? 'border-idpxyz-accent bg-idpxyz-accent/15 text-idpxyz-textBright font-medium'
                          : 'border-idpxyz-border bg-idpxyz-editor text-idpxyz-text hover:bg-idpxyz-hover'
                      }`}
                    >
                      {s.label}
                    </button>
                  );
                })}
              </div>
            ) : null}
          </aside>

          <div className="flex-1 overflow-y-auto p-6 space-y-4">
            {station === 'inbound' ? (
              <div className="flex flex-wrap gap-1.5">
                {inboundIds.map((id) => (
                  <button
                    key={id}
                    type="button"
                    className={tabButtonClass(activeSpecId === id)}
                    onClick={() => prefillForm(id, {})}
                  >
                    {specById(id).label}
                  </button>
                ))}
              </div>
            ) : null}

            {station === 'pack' ? (
              <div className="flex flex-wrap gap-1.5">
                {packIds.map((id) => (
                  <button
                    key={id}
                    type="button"
                    className={tabButtonClass(activeSpecId === id)}
                    onClick={() => prefillForm(id, {})}
                  >
                    {specById(id).label}
                  </button>
                ))}
              </div>
            ) : null}

            {station === 'dispatch' ? (
              <div className="flex flex-wrap gap-1.5">
                {dispatchIds.map((id) => (
                  <button
                    key={id}
                    type="button"
                    className={tabButtonClass(activeSpecId === id)}
                    onClick={() => prefillForm(id, {})}
                  >
                    {specById(id).label}
                  </button>
                ))}
              </div>
            ) : null}

            {activeTarget ? (
              <GuidedFormBody
                key={`${activeTarget.id}-${seed?.nonce ?? 0}`}
                moduleId="warehouse-workstation"
                target={activeTarget}
                prefill={seed?.targetId === activeTarget.id ? seed.values : undefined}
                lookups={lookups}
                defaultWarehouseCode={activeWarehouse}
                problemNote={workFactProblemNote}
                onSubmitted={() => {
                  loadQueues();
                }}
              />
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
}
