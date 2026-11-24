import { useEffect, useMemo, useState } from 'react';
import { Button, Card, CardContent, Input, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { postWorkFact, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import { listWorkDevices, type WorkDeviceRecord } from '../work-devices/api';
import { asWarehouseAction, type WarehouseActionBody } from './actions';
import { GuidedActionPanel, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormSeed } from './guided-forms';
import {
  workFactCategories,
  workFactOutcomeLabels,
  workFactPrefillFor,
  workFactProblemNote,
  workFactSpecs,
  type WorkFactCategory,
} from './work-fact';

const info = moduleInfoById['warehouse-workstation'];

const seqStoragePrefix = 'idp-parcel.web-pda.seq.';

function readSequence(deviceId: string): number {
  try {
    const raw = window.localStorage.getItem(seqStoragePrefix + deviceId);
    const parsed = raw ? Number.parseInt(raw, 10) : 1;
    return Number.isSafeInteger(parsed) && parsed >= 1 ? parsed : 1;
  } catch {
    return 1;
  }
}

function saveSequence(deviceId: string, next: number): void {
  try {
    window.localStorage.setItem(seqStoragePrefix + deviceId, String(next));
  } catch {
    // 浏览器禁用本地存储时只走当前会话递增。
  }
}

interface RecentSubmission {
  at: string;
  label: string;
  endpoint: string;
  factId: string;
  outcome: string;
}

const selectClass =
  'rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 type-value text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

/**
 * 仓内作业台（模拟 PDA / 扫码客户端）。
 * 面向没有实体手持终端的场景：绑定一个本租户已登记在用的 WAREHOUSE 作业设备后，
 * 附带 Parcel-Device-Id 与 Parcel-Work-Scope-* 请求头直接调用全部 18 个作业事实口，
 * 端侧按 ADR-0023 自动签发唯一事实标识与递增设备序号，不绕过服务端任何权限与设备门禁。
 */
export function WarehouseWorkstationPage() {
  const [devices, setDevices] = useState<WorkDeviceRecord[]>([]);
  const [deviceState, setDeviceState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [deviceError, setDeviceError] = useState('');
  const [selectedDeviceId, setSelectedDeviceId] = useState('');
  const [category, setCategory] = useState<WorkFactCategory>('inbound');
  const [sequence, setSequence] = useState(1);
  const [nonce, setNonce] = useState(1);
  const [scanInput, setScanInput] = useState('');
  const [lastScanned, setLastScanned] = useState('');
  const [recent, setRecent] = useState<RecentSubmission[]>([]);

  const loadDevices = () => {
    setDeviceState('loading');
    setDeviceError('');
    void listWorkDevices({ filters: { status: ['REGISTERED'] } }).then((answer) => {
      if (answer.kind !== 'outcome') {
        setDeviceState('error');
        setDeviceError(
          answer.kind === 'transport'
            ? answer.message
            : '读不到扫描设备册（需要登录并持有查阅授予，或部署尚未配置访问渠道）。',
        );
        return;
      }
      const warehouseDevices = answer.body.devices.filter(
        (item) => item.status === 'REGISTERED' && item.workScopeKind === 'WAREHOUSE',
      );
      setDevices(warehouseDevices);
      setDeviceState('ready');
      setSelectedDeviceId((current) => {
        if (warehouseDevices.some((item) => item.deviceId === current)) return current;
        const first = warehouseDevices[0]?.deviceId ?? '';
        if (first) setSequence(readSequence(first));
        return first;
      });
    });
  };

  useEffect(() => {
    loadDevices();
  }, []);

  const activeDevice = devices.find((item) => item.deviceId === selectedDeviceId) ?? null;

  const handleSelectDevice = (nextId: string) => {
    setSelectedDeviceId(nextId);
    if (nextId) {
      setSequence(readSequence(nextId));
      setNonce((n) => n + 1);
    }
  };

  const categorySpecs = useMemo(
    () => workFactSpecs.filter((spec) => spec.category === category),
    [category],
  );

  const targets = useMemo<GuidedActionTarget[]>(() => {
    if (!activeDevice) return [];
    const device = activeDevice;
    return categorySpecs.map((spec) => ({
      id: spec.id,
      label: spec.label,
      title: spec.title,
      endpoint: `POST ${spec.endpoint}`,
      snapshotHint: spec.snapshotHint,
      outcomeLabels: workFactOutcomeLabels,
      form: spec.form,
      submit: async (snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> => {
        const body =
          spec.fixedPayload && typeof snapshot === 'object' && snapshot !== null && !Array.isArray(snapshot)
            ? { ...(snapshot as Record<string, unknown>), ...spec.fixedPayload }
            : snapshot;
        const answer = await postWorkFact<WarehouseActionBody>(spec.endpoint, body, {
          deviceId: device.deviceId,
          scopeKind: 'WAREHOUSE',
          scopeReference: device.workScopeRef,
        });
        const nextSeq = sequence + 1;
        setSequence(nextSeq);
        saveSequence(device.deviceId, nextSeq);
        const factId =
          typeof body === 'object' && body !== null && 'factId' in body
            ? String((body as { factId?: unknown }).factId ?? '')
            : '';
        setRecent((prev) => [
          {
            at: new Date().toISOString().slice(11, 19),
            label: spec.label,
            endpoint: spec.endpoint,
            factId,
            outcome: answer.kind === 'outcome' ? answer.body.outcome : answer.kind,
          },
          ...prev.slice(0, 7),
        ]);
        if (answer.kind !== 'outcome') return answer;
        return { kind: 'outcome', status: answer.status, body: asWarehouseAction(answer.body) };
      },
    }));
  }, [activeDevice, categorySpecs, sequence]);

  const activeSeed = useMemo<GuidedFormSeed | null>(() => {
    if (!activeDevice || categorySpecs.length === 0) return null;
    const first = categorySpecs[0];
    return {
      targetId: first.id,
      values: workFactPrefillFor(first, activeDevice.deviceId, activeDevice.workScopeRef, sequence),
      nonce,
    };
  }, [activeDevice, categorySpecs, nonce, sequence]);

  const copyScanned = () => {
    const trimmed = scanInput.trim();
    if (!trimmed) return;
    setLastScanned(trimmed);
    setScanInput('');
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      void navigator.clipboard.writeText(trimmed).catch(() => undefined);
    }
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <div className="px-4 pt-4 pb-3 border-b border-idpxyz-border shrink-0 space-y-3 bg-idpxyz-panel">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h2 className="type-title font-medium text-idpxyz-text">{info.title}</h2>
            <p className="mt-0.5 type-body text-idpxyz-textMuted">
              网页模拟手持终端（PDA / USB 扫码枪）：绑定一个已登记在用的仓库设备号后直接提交全部 18 类一线作业事实。操作者须持有该仓的「作业事实登记」授予。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={loadDevices}>
              刷新在用设备
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#work-devices'; }}>
              去扫描设备登记工位
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#operator-access'; }}>
              去操作者与授予
            </Button>
          </div>
        </div>

        {deviceState === 'loading' ? (
          <p className="type-body text-idpxyz-textMuted">正在读取本租户在用的仓库作业设备…</p>
        ) : deviceState === 'error' ? (
          <p className="type-body text-idpxyz-danger">{deviceError}</p>
        ) : devices.length === 0 ? (
          <Card>
            <CardContent className="pt-4 space-y-2">
              <p className="type-body font-medium text-idpxyz-text">当前租户还没有已登记在用的仓库（WAREHOUSE）作业设备</p>
              <p className="type-body text-idpxyz-textMuted">
                按作业事实门禁，模拟终端不能使用假设备号或默认设备号。没有实体 PDA 时，请管理员先到「扫描设备 → 登记与停用」登记一个网页工位设备号（作业范围选仓库，地点编号填仓编码），再到「操作者与授予」为当前操作员授予该仓的作业事实登记权限。
              </p>
            </CardContent>
          </Card>
        ) : (
          <div className="flex flex-wrap items-center gap-3">
            <label className="inline-flex items-center gap-2 type-body text-idpxyz-textMuted">
              <span>当前模拟设备</span>
              <select
                className={selectClass}
                value={selectedDeviceId}
                onChange={(event) => handleSelectDevice(event.target.value)}
              >
                {devices.map((item) => (
                  <option key={item.deviceId} value={item.deviceId}>
                    {`${item.deviceId}（仓 ${item.workScopeRef}）`}
                  </option>
                ))}
              </select>
            </label>
            {activeDevice ? (
              <span className="font-mono type-caption text-idpxyz-textMuted">
                {`绑定仓：${activeDevice.workScopeRef} · 下一设备序号：#${sequence}`}
              </span>
            ) : null}
            <Button type="button" variant="outline" onClick={() => setNonce((n) => n + 1)}>
              重签新事实号与此刻
            </Button>
            <div className="flex items-center gap-1.5 ml-auto">
              <Input
                className="w-56 font-mono"
                value={scanInput}
                placeholder="USB 扫码枪扫描回车…"
                onChange={(event) => setScanInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault();
                    copyScanned();
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={copyScanned}>
                记下条码
              </Button>
            </div>
          </div>
        )}

        {lastScanned ? (
          <p className="type-caption text-idpxyz-textMuted">
            最近扫码：<span className="font-mono text-idpxyz-text">{lastScanned}</span>（已尝试复制到剪贴板，可直接粘贴到下方表单字段）
          </p>
        ) : null}
      </div>

      {activeDevice ? (
        <Tabs
          value={category}
          onValueChange={(next) => {
            setCategory(next as WorkFactCategory);
            setNonce((n) => n + 1);
          }}
          className="flex-1 flex flex-col overflow-hidden gap-0"
        >
          <TabsList className="px-4 shrink-0">
            {workFactCategories.map((item) => (
              <TabsTrigger key={item.id} value={item.id}>
                {item.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value={category} className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <GuidedActionPanel
              moduleId="warehouse-workstation"
              targets={targets}
              problemNote={workFactProblemNote}
              seed={activeSeed}
              defaultValuesFor={(targetId) => {
                const found = categorySpecs.find((spec) => spec.id === targetId);
                return found ? workFactPrefillFor(found, activeDevice.deviceId, activeDevice.workScopeRef, sequence) : {};
              }}
            />
          </TabsContent>
        </Tabs>
      ) : null}

      {recent.length > 0 ? (
        <div className="px-4 py-2 border-t border-idpxyz-border bg-idpxyz-panel shrink-0">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 type-caption text-idpxyz-textMuted">
            <span className="font-medium text-idpxyz-text">本机最近提交：</span>
            {recent.map((item, index) => (
              <span key={`${item.factId}-${index}`} className="font-mono">
                {`${item.at} ${item.label} [${item.factId}] → ${workFactOutcomeLabels[item.outcome] ?? item.outcome}`}
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
