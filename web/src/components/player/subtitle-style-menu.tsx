import { useLayoutEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CheckIcon, MinusIcon, PlusIcon, TypeIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  applySubtitleStyle,
  persistSubtitleStyle,
  readSubtitleStyle,
  stepSubtitleSize,
  SUBTITLE_BACKGROUNDS,
  SUBTITLE_COLORS,
  SUBTITLE_OUTLINES,
  SUBTITLE_SIZE_MAX,
  SUBTITLE_SIZE_MIN,
  type SubtitleStyle,
} from "@/components/player/subtitle-style";

/**
 * Caption appearance lives outside Video.js: v10 only exposes track select/toggle, not cue
 * cosmetics. Preferences are written to localStorage and injected as concrete `::cue` CSS.
 */
export function SubtitleStyleMenu() {
  const { t } = useTranslation();
  const [style, setStyle] = useState<SubtitleStyle>(readSubtitleStyle);

  useLayoutEffect(() => {
    applySubtitleStyle(style);
    persistSubtitleStyle(style);
  }, [style]);

  const section = <K extends Exclude<keyof SubtitleStyle, "sizePx">>(
    labelKey: string,
    key: K,
    options: readonly { id: SubtitleStyle[K]; labelKey: string }[],
  ) => (
    <>
      <DropdownMenuLabel>{t(labelKey)}</DropdownMenuLabel>
      {options.map((option) => (
        <DropdownMenuItem
          key={String(option.id)}
          onClick={() => setStyle((current) => ({ ...current, [key]: option.id }))}
        >
          <span className="flex-1">{t(option.labelKey)}</span>
          {style[key] === option.id ? <CheckIcon className="size-4 opacity-70" /> : null}
        </DropdownMenuItem>
      ))}
    </>
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="px-2" aria-label={t("player.subtitleStyle")}>
          <TypeIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
        <DropdownMenuLabel>{t("player.subtitleSize")}</DropdownMenuLabel>
        <div className="flex items-center justify-between gap-2 px-2 py-1.5 md:gap-3 md:px-3 md:py-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("player.subtitleSizeDecrease")}
            disabled={style.sizePx <= SUBTITLE_SIZE_MIN}
            onPointerDown={(event) => event.preventDefault()}
            onClick={(event) => {
              event.preventDefault();
              setStyle((current) => ({
                ...current,
                sizePx: stepSubtitleSize(current.sizePx, -1),
              }));
            }}
          >
            <MinusIcon />
          </Button>
          <span className="min-w-12 text-center text-sm tabular-nums md:min-w-14 md:text-base">
            {t("player.subtitleSizeValue", { size: style.sizePx })}
          </span>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("player.subtitleSizeIncrease")}
            disabled={style.sizePx >= SUBTITLE_SIZE_MAX}
            onPointerDown={(event) => event.preventDefault()}
            onClick={(event) => {
              event.preventDefault();
              setStyle((current) => ({
                ...current,
                sizePx: stepSubtitleSize(current.sizePx, 1),
              }));
            }}
          >
            <PlusIcon />
          </Button>
        </div>
        <DropdownMenuSeparator />
        {section("player.subtitleColor", "color", SUBTITLE_COLORS)}
        <DropdownMenuSeparator />
        {section("player.subtitleOutline", "outline", SUBTITLE_OUTLINES)}
        <DropdownMenuSeparator />
        {section("player.subtitleBackground", "background", SUBTITLE_BACKGROUNDS)}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
