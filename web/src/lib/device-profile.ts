/**
 * Jellyfin-style browser device profile for FI-8 Direct Play negotiate.
 * Rebuilt on each play / renegotiate via hidden <video>.canPlayType.
 */

export type DirectPlayProfile = {
  container: string;
  videoCodec?: string;
  audioCodec?: string;
};

export type DeviceProfile = {
  maxStaticBitrate: number;
  directPlay: DirectPlayProfile[];
  preferHls: boolean;
  subtitle: string[];
};

const DEFAULT_MAX_BITRATE = 120_000_000;

type CodecProbe = {
  container: string;
  videoCodec: string;
  audioCodec: string;
  mime: string;
};

const PROBES: CodecProbe[] = [
  {
    container: "mp4",
    videoCodec: "h264",
    audioCodec: "aac",
    mime: 'video/mp4; codecs="avc1.42E01E, mp4a.40.2"',
  },
  {
    container: "mp4",
    videoCodec: "h264",
    audioCodec: "ac3",
    mime: 'video/mp4; codecs="avc1.42E01E, ac-3"',
  },
  {
    container: "mp4",
    videoCodec: "hevc",
    audioCodec: "aac",
    mime: 'video/mp4; codecs="hvc1.1.6.L93.B0, mp4a.40.2"',
  },
  {
    container: "webm",
    videoCodec: "vp9",
    audioCodec: "opus",
    mime: 'video/webm; codecs="vp9, opus"',
  },
  {
    container: "webm",
    videoCodec: "vp8",
    audioCodec: "vorbis",
    mime: 'video/webm; codecs="vp8, vorbis"',
  },
  {
    container: "webm",
    videoCodec: "av1",
    audioCodec: "opus",
    mime: 'video/webm; codecs="av01.0.05M.08, opus"',
  },
];

function canPlay(video: HTMLVideoElement, mime: string): boolean {
  const result = video.canPlayType(mime);
  return result === "probably" || result === "maybe";
}

function mergeDirectPlay(entries: DirectPlayProfile[]): DirectPlayProfile[] {
  const byContainer = new Map<string, { video: Set<string>; audio: Set<string> }>();

  for (const entry of entries) {
    let bucket = byContainer.get(entry.container);
    if (!bucket) {
      bucket = { video: new Set(), audio: new Set() };
      byContainer.set(entry.container, bucket);
    }
    for (const codec of (entry.videoCodec ?? "").split(",")) {
      const trimmed = codec.trim();
      if (trimmed) {
        bucket.video.add(trimmed);
      }
    }
    for (const codec of (entry.audioCodec ?? "").split(",")) {
      const trimmed = codec.trim();
      if (trimmed) {
        bucket.audio.add(trimmed);
      }
    }
  }

  return [...byContainer.entries()].map(([container, codecs]) => ({
    container,
    videoCodec: [...codecs.video].join(","),
    audioCodec: [...codecs.audio].join(","),
  }));
}

/** Build a fresh device profile from the current browser (call per play). */
export function buildDeviceProfile(): DeviceProfile {
  if (typeof document === "undefined") {
    return {
      maxStaticBitrate: DEFAULT_MAX_BITRATE,
      directPlay: [],
      preferHls: false,
      subtitle: ["external"],
    };
  }

  const video = document.createElement("video");
  const matched: DirectPlayProfile[] = [];

  for (const probe of PROBES) {
    if (canPlay(video, probe.mime)) {
      matched.push({
        container: probe.container,
        videoCodec: probe.videoCodec,
        audioCodec: probe.audioCodec,
      });
    }
  }

  return {
    maxStaticBitrate: DEFAULT_MAX_BITRATE,
    directPlay: mergeDirectPlay(matched),
    preferHls: false,
    subtitle: ["external"],
  };
}
