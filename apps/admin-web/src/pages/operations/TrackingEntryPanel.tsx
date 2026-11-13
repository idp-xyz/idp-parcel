import { useState } from 'react';
import { RegistrationPanel } from '../../components/registration';
import { judgmentProblemNote } from './effective-time-judgment';
import { submitTrackingEntries, trackingEntryEndpoint, trackingEntryHint, trackingEntryLines, trackingEntryOutcomeLabels, trackingEntryTitle } from './tracking-entry';

/**
 * 录入承运商轨迹：登记面板加一段逐条答复。面板自己只示整批那一格答复，一批里各条各答各的，所以在下面逐条列出来。
 */
export function TrackingEntryPanel() {
  const [lines, setLines] = useState<string[]>([]);
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <RegistrationPanel
        moduleId="effective-time-judgment"
        title={trackingEntryTitle}
        endpoint={`POST ${trackingEntryEndpoint}`}
        snapshotHint={trackingEntryHint}
        submit={async (snapshot) => {
          const answer = await submitTrackingEntries(snapshot);
          setLines(answer.kind === 'outcome' ? trackingEntryLines(answer.body) : []);
          return answer;
        }}
        outcomeLabels={trackingEntryOutcomeLabels}
        problemNote={judgmentProblemNote}
      />
      {lines.length > 0 ? (
        <ul className="shrink-0 max-h-48 overflow-auto border-t border-idpxyz-border px-4 py-3 type-body space-y-1" aria-label="各条的结果">
          {lines.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
