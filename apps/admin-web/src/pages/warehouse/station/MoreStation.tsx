import { useMemo, useState } from 'react';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { postWorkFact } from '../../catalogue-api';
import { asWarehouseAction, type WarehouseActionBody } from '../actions';
import { GuidedFormBody, type GuidedActionTarget } from '../GuidedActionPanel';
import { useWarehouseLookups } from '../use-catalogues';
import { workFactOutcomeLabels, workFactPrefillFor, workFactProblemNote, workFactSpecs } from '../work-fact';
import { advanceDeviceSequence, browserStorage, readDeviceSequence } from './device-binding';
import { moreJobIds } from './more-jobs';
import { StationFrame, stationActionClass, stationPrimaryClass } from './StationFrame';

// 十种没有单独工位的作业，沿用原来的引导表单。提交仍走作业事实口，报文由那张表单装，不另写一套。

export function MoreStation({ device, onChangeDevice }: { device: WorkDeviceRecord; onChangeDevice?: () => void }) {
  const lookups = useWarehouseLookups();
  const [jobId, setJobId] = useState<string>(moreJobIds[0]);
  const [nonce, setNonce] = useState(0);
  const spec = workFactSpecs.find((item) => item.id === jobId) ?? workFactSpecs[0];
  const target = useMemo<GuidedActionTarget>(() => {
    const headers = { deviceId: device.deviceId, scopeKind: 'WAREHOUSE', scopeReference: device.workScopeRef };
    return {
      id: spec.id,
      label: spec.label,
      title: spec.title,
      endpoint: `POST ${spec.endpoint}`,
      snapshotHint: spec.snapshotHint,
      outcomeLabels: workFactOutcomeLabels,
      form: spec.form,
      submit: async (snapshot: unknown) => {
        const body = snapshot && typeof snapshot === 'object' ? { ...(spec.fixedPayload ?? {}), ...(snapshot as Record<string, unknown>) } : snapshot;
        let used: number | undefined;
        if (spec.usesDeviceSequence && body && typeof body === 'object') {
          const record = body as Record<string, unknown>;
          used = typeof record.deviceSequence === 'number' ? record.deviceSequence : readDeviceSequence(browserStorage(), device.deviceId) + 1;
          record.deviceSequence = used;
        }
        const answer = await postWorkFact<WarehouseActionBody>(spec.endpoint, body, headers);
        if (answer.kind === 'outcome' && used !== undefined) advanceDeviceSequence(browserStorage(), device.deviceId, used);
        if (answer.kind !== 'outcome') return answer;
        return { kind: 'outcome', status: answer.status, body: asWarehouseAction(answer.body) };
      },
    };
  }, [device.deviceId, device.workScopeRef, spec]);
  const prefill = workFactPrefillFor(spec, device.deviceId, device.workScopeRef, readDeviceSequence(browserStorage(), device.deviceId) + 1);

  return (
    <StationFrame
      title="更多作业"
      device={{ id: device.deviceId, warehouse: device.workScopeRef }}
      onChangeDevice={onChangeDevice}
      left={
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap gap-2">
            {moreJobIds.map((id) => {
              const item = workFactSpecs.find((candidate) => candidate.id === id);
              return (
                <button key={id} type="button" className={id === jobId ? stationPrimaryClass : stationActionClass} onClick={() => { setJobId(id); setNonce((n) => n + 1); }}>
                  {item?.label ?? id}
                </button>
              );
            })}
          </div>
          <GuidedFormBody
            key={`${spec.id}-${nonce}`}
            moduleId="warehouse-workstation"
            target={target}
            prefill={prefill}
            lookups={lookups}
            defaultWarehouseCode={device.workScopeRef}
            problemNote={workFactProblemNote}
            bare
          />
        </div>
      }
    />
  );
}
