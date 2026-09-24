import { selectTime, usePlayer } from "@videojs/react";
import { useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  isInSkipIntroSegment,
  skipIntroSeekSeconds,
  type SkipIntroSegment,
} from "@/lib/skip-intro";

type TimeState = {
  currentTime: number;
  seek: (time: number) => Promise<number>;
};

type SkipIntroActionProps = {
  segment: SkipIntroSegment;
};

/** Skip intro button in the series chrome actions slot (also used for non-series video). */
export function SkipIntroAction({ segment }: SkipIntroActionProps): ReactNode {
  const { t } = useTranslation();
  const time = usePlayer(selectTime) as TimeState | undefined;
  const currentTime = time?.currentTime ?? 0;

  const visible = useMemo(() => isInSkipIntroSegment(segment, currentTime), [segment, currentTime]);

  if (!visible) {
    return null;
  }

  return (
    <button
      type="button"
      className="sudostream-series-chrome__next"
      onClick={() => {
        if (time?.seek) {
          void time.seek(skipIntroSeekSeconds(segment));
        }
      }}
    >
      {t("player.skipIntro")}
    </button>
  );
}
