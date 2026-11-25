import { useCallback, useRef, useState } from 'react';
import { browserStorage } from './device-binding';
import { playCue, readSoundOn, writeSoundOn, type ScanCue } from './scan-sound';

// 提示音的开关：记在本机，下次进来还是这个样子。开关的默认与读写规矩在 scan-sound.ts。

export interface StationSound {
  on: boolean;
  toggle: () => void;
  /** 开着才响；浏览器不让响时 playCue 自己安静地不响。 */
  cue: (cue: ScanCue) => void;
}

export function useStationSound(): StationSound {
  const [on, setOn] = useState(() => readSoundOn(browserStorage()));
  const onRef = useRef(on);
  onRef.current = on;

  const toggle = useCallback(() => {
    const next = !onRef.current;
    onRef.current = next;
    writeSoundOn(browserStorage(), next);
    setOn(next);
  }, []);

  const cue = useCallback((which: ScanCue) => {
    if (onRef.current) playCue(which);
  }, []);

  return { on, toggle, cue };
}
