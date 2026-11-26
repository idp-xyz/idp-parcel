import { statusToneBanner } from '../../../components/status-tone';
import { stationHash } from '../../../shell/station-address';
import { legacyWorkstationHash, stationEntries, type StationEntry } from './stations';
import { stationButtonClass, stationPrimaryClass } from './StationFrame';

// 选工位：列出各个工位。做好的可以进；还在仓内作业台里的，写明这一点并链到那一页——不摆点不动的假按钮。

function StationCard({ entry, highlighted }: { entry: StationEntry; highlighted: boolean }) {
  return (
    <li
      className={`flex flex-col gap-3 rounded-lg border bg-idpxyz-editor p-4 ${
        highlighted ? 'border-[color:var(--ds-status-warning-fg)]' : 'border-idpxyz-border'
      }`}
    >
      <div>
        <h2 className="type-section font-semibold text-idpxyz-textBright">{entry.name}</h2>
        <p className="mt-1 type-body text-idpxyz-textMuted">{entry.summary}</p>
      </div>
      <div className="mt-auto flex flex-wrap items-center gap-3">
        {entry.ready ? (
          <a className={stationPrimaryClass} href={stationHash(entry.id)}>
            进入{entry.name}
          </a>
        ) : (
          <>
            <p className="type-body text-idpxyz-text">还在仓内作业台里</p>
            <a className={stationButtonClass} href={legacyWorkstationHash}>
              去仓内作业台
            </a>
          </>
        )}
      </div>
    </li>
  );
}

export function StationHome({ notice, highlight }: { notice?: string; highlight?: string }) {
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="type-section font-semibold text-idpxyz-textBright">选一个工位</h2>
        <p className="mt-1 type-body text-idpxyz-textMuted">
          每个工位一个地址，工位电脑收藏它，打开就是全屏。全屏的工位逐步做出来，没做好的暂时还在仓内作业台里。
        </p>
      </div>
      {notice ? (
        <p role="status" className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
          {notice}
        </p>
      ) : null}
      <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {stationEntries.map((entry) => (
          <StationCard key={entry.id} entry={entry} highlighted={entry.id === highlight} />
        ))}
      </ul>
    </div>
  );
}
