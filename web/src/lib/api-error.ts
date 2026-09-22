export function extractApiErrorMessage(error: unknown): string | undefined {
  if (!error || typeof error !== "object") {
    return undefined;
  }

  if ("error" in error && typeof error.error === "string") {
    return error.error;
  }

  return undefined;
}
