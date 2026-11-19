import { selectTime, usePlayer } from "@videojs/react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
  /** When true, seek past intro once on enter (E-45). */
  autoSkip?: boolean;
};

const UNDO_MS = 5000;

type UndoState = {
  fromSeconds: number;
  expiresAt: number;
};

/** Skip intro button + optional auto-skip / Undo toast / keyboard S. */
export function SkipIntroAction({ segment, autoSkip = false }: SkipIntroActionProps): ReactNode {
  const { t } = useTranslation();
  const time = usePlayer(selectTime) as TimeState | undefined;
  const currentTime = time?.currentTime ?? 0;
  const timeRef = useRef(time);
  const autoFiredRef = useRef(false);
  const inIntroRef = useRef(false);
  const [undo, setUndo] = useState<UndoState | null>(null);

  useEffect(() => {
    timeRef.current = time;
  }, [time]);

  const seekPastIntro = useCallback(
    (showUndo: boolean) => {
      const active = timeRef.current;
      if (!active?.seek) {
        return;
      }
      const fromSeconds = active.currentTime;
      void active.seek(skipIntroSeekSeconds(segment));
      if (showUndo) {
        setUndo({ fromSeconds, expiresAt: Date.now() + UNDO_MS });
      }
    },
    [segment],
  );

  const visible = useMemo(() => isInSkipIntroSegment(segment, currentTime), [segment, currentTime]);

  useEffect(() => {
    const wasIn = inIntroRef.current;
    inIntroRef.current = visible;
    if (wasIn && !visible) {
      autoFiredRef.current = false;
    }
  }, [visible]);

  useEffect(() => {
    if (!autoSkip || !visible || autoFiredRef.current) {
      return;
    }
    autoFiredRef.current = true;
    seekPastIntro(true);
  }, [autoSkip, visible, seekPastIntro]);

  useEffect(() => {
    if (!undo) {
      return;
    }
    const remaining = Math.max(0, undo.expiresAt - Date.now());
    const timer = window.setTimeout(() => setUndo(null), remaining);
    return () => window.clearTimeout(timer);
  }, [undo]);

  useEffect(() => {
    if (!visible) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "s" && event.key !== "S") {
        return;
      }
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable)
      ) {
        return;
      }
      event.preventDefault();
      seekPastIntro(false);
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [visible, seekPastIntro]);

  return (
    <>
      {visible ? (
        <button
          type="button"
          className="sudostream-series-chrome__next"
          onClick={() => seekPastIntro(false)}
        >
          {t("player.skipIntro")}
        </button>
      ) : null}
      {undo ? (
        <div className="sudostream-skip-undo" role="status">
          <span>{t("player.autoSkipIntroToast")}</span>
          <button
            type="button"
            className="sudostream-skip-undo__action"
            onClick={() => {
              const active = timeRef.current;
              if (active?.seek) {
                void active.seek(undo.fromSeconds);
              }
              setUndo(null);
            }}
          >
            {t("player.autoSkipIntroUndo")}
          </button>
        </div>
      ) : null}
    </>
  );
}
