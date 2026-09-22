/** Build a WebVTT chapters document for a native `<track kind="chapters">`. */
export function chaptersToWebVTT(
  chapters: Array<{ startSeconds: number; endSeconds: number; title: string }>,
): string {
  const lines = ["WEBVTT", ""];
  for (const chapter of chapters) {
    lines.push(
      `${formatVttTimestamp(chapter.startSeconds)} --> ${formatVttTimestamp(chapter.endSeconds)}`,
    );
    lines.push(chapter.title || "Chapter");
    lines.push("");
  }
  return lines.join("\n");
}

function formatVttTimestamp(seconds: number): string {
  const totalMs = Math.max(0, Math.round(seconds * 1000));
  const hours = Math.floor(totalMs / 3_600_000);
  const minutes = Math.floor((totalMs % 3_600_000) / 60_000);
  const secs = Math.floor((totalMs % 60_000) / 1000);
  const ms = totalMs % 1000;
  return `${pad(hours, 2)}:${pad(minutes, 2)}:${pad(secs, 2)}.${pad(ms, 3)}`;
}

function pad(value: number, width: number): string {
  return String(value).padStart(width, "0");
}
