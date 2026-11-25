import { enterStation } from '../../shell/enter-station';
import { stationHash } from '../../shell/station-address';
import { stationEntries } from './station/stations';

// 仓内作业台不再摆表单。六个工位和其余十种作业都在不带外壳的全屏作业台里，这一页只负责进去。

export function WarehouseWorkstationPage() {
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="type-title text-idpxyz-textBright">仓内作业台</h1>
        <p className="mt-1 type-body text-idpxyz-textMuted">作业改在全屏工位里做。选一个进去；这一页不再填写作业。</p>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {stationEntries.map((entry) => (
          <button key={entry.id} type="button" className="rounded border border-idpxyz-border bg-idpxyz-editor p-4 text-left hover:bg-idpxyz-hover" onClick={() => enterStation(stationHash(entry.id))}>
            <div className="type-body font-semibold text-idpxyz-textBright">{entry.name}</div>
            <div className="mt-1 type-caption text-idpxyz-textMuted">{entry.ready ? entry.summary : '还在全屏作业台的更多作业里做。'}</div>
          </button>
        ))}
        <button type="button" className="rounded border border-idpxyz-border bg-idpxyz-editor p-4 text-left hover:bg-idpxyz-hover" onClick={() => enterStation(stationHash('more'))}>
          <div className="type-body font-semibold text-idpxyz-textBright">更多作业</div>
          <div className="mt-1 type-caption text-idpxyz-textMuted">仓测、待认领、退货到仓、形态与状态转换、短少、加工换标、开箱、截单回库。</div>
        </button>
      </div>
    </div>
  );
}
