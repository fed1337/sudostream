import { encodeMediaPath } from "@/lib/media-urls";

/**
 * Catch-all `{path}` OpenAPI params are percent-encoded as a single segment by
 * hey-api (`a/b` → `a%2Fb`). Gin `*path` needs literal `/` separators, so build
 * the URL with per-segment encoding and skip the `{path}` template.
 */
export function catchAllUrl(prefix: string, relPath: string): string {
  return `${prefix}/${encodeMediaPath(relPath)}`;
}
