import { defineConfig } from "@hey-api/openapi-ts";

export default defineConfig({
  input: "../openapi/swagger.json",
  output: "src/client",
  plugins: [
    {
      name: "@hey-api/client-ky",
      runtimeConfigPath: "./src/api/hey-api.ts",
    },
    "@hey-api/typescript",
    "@hey-api/sdk",
    {
      name: "@hey-api/schemas",
      type: "json",
    },
    "zod",
    "@tanstack/react-query",
  ],
});
