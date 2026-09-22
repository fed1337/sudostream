import type { CreateClientConfig } from "@/client/client.gen";

const host = import.meta.env.VITE_HOST ?? "";

/**
 * Initial Hey API client config. Auth interceptors are attached in `./client.ts`
 * after the generated client is created.
 */
export const createClientConfig: CreateClientConfig = (config) => ({
  ...config,
  baseUrl: host,
  credentials: "include",
});
