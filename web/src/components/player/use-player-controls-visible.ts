import { selectControls, usePlayer } from "@videojs/react";

type ControlsState = {
  controlsVisible?: boolean;
};

/** True while VideoSkin controls are shown; defaults visible before controls feature is ready. */
export function usePlayerControlsVisible(): boolean {
  const controls = usePlayer(selectControls) as ControlsState | undefined;
  return controls?.controlsVisible ?? true;
}
