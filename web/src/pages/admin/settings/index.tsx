import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { CpuIcon, NetworkIcon, SettingsIcon, Trash2Icon, TvIcon } from "lucide-react";
import { z } from "zod";

import type {
  SudoStreamInternalAuthSettings,
  SudoStreamInternalDlnaSettings,
  SudoStreamInternalNetworkProbeResult,
  SudoStreamInternalNetworkSettings,
  SudoStreamInternalTranscodeTranscodeSettings,
} from "@/client/types.gen";
import {
  getApiAdminDlnaSettingsOptions,
  getApiAdminNetworkSettingsOptions,
  getApiAdminSettingsOptions,
  getApiAdminTranscodeSettingsOptions,
  getApiAdminTrashSettingsOptions,
  getApiAdminUsersOptions,
} from "@/client/@tanstack/react-query.gen";
import {
  patchApiAdminDlnaSettings,
  patchApiAdminNetworkSettings,
  patchApiAdminSettings,
  patchApiAdminTranscodeSettings,
  patchApiAdminTrashSettings,
  postApiAdminNetworkSettingsTest,
} from "@/client/sdk.gen";
import { TrashSettingsForm, type TrashSettingsValues } from "@/pages/admin/trash-settings-form";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { FieldHelp } from "@/components/field-help";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { extractApiErrorMessage, translateApiError } from "@/lib/api-error";

type Settings = SudoStreamInternalAuthSettings;
type TranscodeSettings = SudoStreamInternalTranscodeTranscodeSettings;
type NetworkSettings = SudoStreamInternalNetworkSettings;
type DlnaSettings = SudoStreamInternalDlnaSettings;

type HwAccelValue = NonNullable<TranscodeSettings["hwAccel"]>;
type DownmixValue = NonNullable<TranscodeSettings["downmixAlgorithm"]>;
type ToneAlgoValue = NonNullable<TranscodeSettings["toneMappingAlgorithm"]>;

const hwAccelOptions: HwAccelValue[] = ["off", "qsv", "vaapi", "nvenc", "rockchip"];

export default function AdminSettingsPage() {
  const { t } = useTranslation();
  const settingsQuery = useQuery({
    ...getApiAdminSettingsOptions(),
  });
  const transcodeQuery = useQuery({
    ...getApiAdminTranscodeSettingsOptions(),
  });
  const networkQuery = useQuery({
    ...getApiAdminNetworkSettingsOptions(),
  });
  const dlnaQuery = useQuery({
    ...getApiAdminDlnaSettingsOptions(),
  });
  const trashSettingsQuery = useQuery({
    ...getApiAdminTrashSettingsOptions(),
  });
  const usersQuery = useQuery({
    ...getApiAdminUsersOptions(),
  });

  const form = useForm<Settings>({
    defaultValues: {},
  });

  const networkSchema = useMemo(
    () =>
      z.object({
        proxyUrl: z.string(),
        proxyUser: z.string(),
        proxyPassword: z.string(),
        noProxy: z.string(),
      }),
    [],
  );
  type NetworkValues = z.infer<typeof networkSchema>;

  const networkForm = useForm<NetworkValues>({
    resolver: zodResolver(networkSchema),
    defaultValues: {
      proxyUrl: "",
      proxyUser: "",
      proxyPassword: "",
      noProxy: "",
    },
  });

  const transcodeSchema = useMemo(
    () =>
      z.object({
        hwAccel: z.enum(["off", "qsv", "vaapi", "nvenc", "rockchip"]),
        downmixAlgorithm: z.enum(["ac4", "none", "dave750", "nightmodeDialogue", "rfc7845"]),
        downmixBoost: z.string().refine(
          (value) => {
            if (value.trim() === "") {
              return true;
            }
            const n = Number(value);
            return Number.isFinite(n) && n >= 0.5 && n <= 3;
          },
          { error: t("apiErrors.invalidDownmixBoostValue") },
        ),
        toneMappingEnabled: z.boolean(),
        toneMappingAlgorithm: z.enum(["bt2390", "hable", "reinhard", "mobius"]),
      }),
    [t],
  );
  type TranscodeValues = z.infer<typeof transcodeSchema>;

  const transcodeForm = useForm<TranscodeValues>({
    resolver: zodResolver(transcodeSchema),
    defaultValues: {
      hwAccel: "off",
      downmixAlgorithm: "ac4",
      downmixBoost: "1",
      toneMappingEnabled: true,
      toneMappingAlgorithm: "bt2390",
    },
  });

  const dlnaSchema = useMemo(
    () =>
      z
        .object({
          enabled: z.boolean(),
          userId: z.string(),
        })
        .superRefine((data, ctx) => {
          if (data.enabled && data.userId.trim() === "") {
            ctx.addIssue({
              code: "custom",
              path: ["userId"],
              message: t("apiErrors.dlnaTvUserIdIsRequiredWhenEnabled"),
            });
          }
        }),
    [t],
  );
  type DlnaValues = z.infer<typeof dlnaSchema>;

  const dlnaForm = useForm<DlnaValues>({
    resolver: zodResolver(dlnaSchema),
    defaultValues: { enabled: false, userId: "" },
  });

  const [hasProxyPassword, setHasProxyPassword] = useState(false);
  const [probeResult, setProbeResult] = useState<SudoStreamInternalNetworkProbeResult | null>(null);

  useEffect(() => {
    if (settingsQuery.data) {
      form.reset(settingsQuery.data);
    }
  }, [form, settingsQuery.data]);

  useEffect(() => {
    if (networkQuery.data) {
      networkForm.reset({
        proxyUrl: networkQuery.data.proxyUrl ?? "",
        proxyUser: networkQuery.data.proxyUser ?? "",
        proxyPassword: "",
        noProxy: networkQuery.data.noProxy ?? "",
      });
      setHasProxyPassword(Boolean(networkQuery.data.hasProxyPassword));
    }
  }, [networkForm, networkQuery.data]);

  useEffect(() => {
    if (transcodeQuery.data) {
      transcodeForm.reset({
        hwAccel: (transcodeQuery.data.hwAccel ?? "off") as HwAccelValue,
        downmixAlgorithm: (transcodeQuery.data.downmixAlgorithm ?? "ac4") as DownmixValue,
        downmixBoost:
          typeof transcodeQuery.data.downmixBoost === "number"
            ? String(transcodeQuery.data.downmixBoost)
            : "",
        toneMappingEnabled: Boolean(transcodeQuery.data.toneMappingEnabled ?? true),
        toneMappingAlgorithm: (transcodeQuery.data.toneMappingAlgorithm ??
          "bt2390") as ToneAlgoValue,
      });
    }
  }, [transcodeForm, transcodeQuery.data]);

  useEffect(() => {
    if (dlnaQuery.data) {
      dlnaForm.reset({
        enabled: Boolean(dlnaQuery.data.enabled),
        userId: dlnaQuery.data.userId ?? "",
      });
    }
  }, [dlnaForm, dlnaQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async (values: Settings) => {
      const response = await patchApiAdminSettings({
        body: values,
      });

      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.saveSettingsFailed"));
      }

      return response.data;
    },
    onSuccess: (data) => {
      form.reset(data);
    },
  });

  const saveTranscodeMutation = useMutation({
    mutationFn: async (body: TranscodeSettings) => {
      const response = await patchApiAdminTranscodeSettings({
        body,
      });

      if (response.error || !response.data) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.saveTranscodeSettingsFailed"),
        );
      }

      return response.data;
    },
    onSuccess: (data) => {
      transcodeForm.reset({
        hwAccel: (data.hwAccel ?? "off") as HwAccelValue,
        downmixAlgorithm: (data.downmixAlgorithm ?? "ac4") as DownmixValue,
        downmixBoost: typeof data.downmixBoost === "number" ? String(data.downmixBoost) : "",
        toneMappingEnabled: Boolean(data.toneMappingEnabled ?? true),
        toneMappingAlgorithm: (data.toneMappingAlgorithm ?? "bt2390") as ToneAlgoValue,
      });
    },
  });

  const toNetworkBody = (values: NetworkValues): NetworkSettings => ({
    proxyUrl: values.proxyUrl,
    proxyUser: values.proxyUser || undefined,
    proxyPassword: values.proxyPassword || undefined,
    noProxy: values.noProxy,
  });

  const saveNetworkMutation = useMutation({
    mutationFn: async (body: NetworkSettings) => {
      const response = await patchApiAdminNetworkSettings({ body });
      if (response.error || !response.data) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.saveNetworkSettingsFailed"),
        );
      }

      return response.data;
    },
    onSuccess: (data) => {
      networkForm.reset({
        proxyUrl: data.proxyUrl ?? "",
        proxyUser: data.proxyUser ?? "",
        proxyPassword: "",
        noProxy: data.noProxy ?? "",
      });
      setHasProxyPassword(Boolean(data.hasProxyPassword));
      setProbeResult(null);
      void networkQuery.refetch();
    },
  });

  const saveDlnaMutation = useMutation({
    mutationFn: async (body: DlnaSettings) => {
      const response = await patchApiAdminDlnaSettings({ body });
      if (response.error || !response.data) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.saveDLNASettingsFailed"),
        );
      }

      return response.data;
    },
    onSuccess: (data) => {
      dlnaForm.reset({
        enabled: Boolean(data.enabled),
        userId: data.userId ?? "",
      });
      void dlnaQuery.refetch();
    },
  });

  const saveTrashMutation = useMutation({
    mutationFn: async (body: TrashSettingsValues) => {
      const response = await patchApiAdminTrashSettings({ body });
      if (response.error || !response.data) {
        throw new Error(extractApiErrorMessage(response.error) ?? t("admin.trashSettingsFailed"));
      }

      return response.data;
    },
    onSuccess: () => {
      void trashSettingsQuery.refetch();
    },
  });

  const tvUsers = (usersQuery.data?.users ?? []).filter((user) => user.role === "tv");

  const testNetworkMutation = useMutation({
    mutationFn: async (body: NetworkSettings) => {
      const response = await postApiAdminNetworkSettingsTest({ body });
      if (response.error || !response.data) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("admin.testNetworkSettingsFailed"),
        );
      }

      return response.data;
    },
    onSuccess: (data) => {
      setProbeResult(data);
    },
  });

  if (
    settingsQuery.isLoading ||
    transcodeQuery.isLoading ||
    networkQuery.isLoading ||
    trashSettingsQuery.isLoading
  ) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-96 w-full rounded-xl" />
      </div>
    );
  }

  const toneMappingEnabled = transcodeForm.watch("toneMappingEnabled");
  const networkErrors = networkForm.formState.errors;
  const transcodeErrors = transcodeForm.formState.errors;
  const dlnaErrors = dlnaForm.formState.errors;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("admin.settingsTitle")}</h1>
        <p className="text-muted-foreground">{t("admin.settingsDescription")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <SettingsIcon className="size-5" />
            {t("admin.authSettings")}
          </CardTitle>
          <CardDescription>{t("admin.authSettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={form.handleSubmit((values) => {
              saveMutation.reset();
              saveMutation.mutate(values);
            })}
          >
            {saveMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.saveSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {saveMutation.error instanceof Error
                    ? translateApiError(saveMutation.error, t, {
                        fallbackKey: "admin.saveSettingsFailed",
                      })
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}

            {saveMutation.isSuccess ? (
              <Alert variant="success">
                <AlertTitle>{t("admin.saveSettingsSuccess")}</AlertTitle>
              </Alert>
            ) : null}

            <FieldGroup>
              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Controller
                    control={form.control}
                    name="emailConfirmationRequired"
                    render={({ field }) => (
                      <Switch
                        id="emailConfirmationRequired"
                        checked={Boolean(field.value)}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <FieldLabel htmlFor="emailConfirmationRequired">
                    {t("admin.emailConfirmationRequired")}
                  </FieldLabel>
                </div>
              </Field>

              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Controller
                    control={form.control}
                    name="twoFactorRequired"
                    render={({ field }) => (
                      <Switch
                        id="twoFactorRequired"
                        checked={Boolean(field.value)}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <FieldLabel htmlFor="twoFactorRequired">
                    {t("admin.twoFactorRequired")}
                  </FieldLabel>
                </div>
              </Field>

              <Field>
                <Button type="submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                  {saveMutation.isPending ? t("admin.saving") : t("admin.saveSettings")}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <NetworkIcon className="size-5" />
            {t("admin.networkSettings")}
          </CardTitle>
          <CardDescription>{t("admin.networkSettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={networkForm.handleSubmit((values) => {
              saveNetworkMutation.reset();
              testNetworkMutation.reset();
              setProbeResult(null);
              saveNetworkMutation.mutate(toNetworkBody(values));
            })}
          >
            {saveNetworkMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.saveNetworkSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {saveNetworkMutation.error instanceof Error
                    ? translateApiError(saveNetworkMutation.error, t, {
                        fallbackKey: "admin.saveNetworkSettingsFailed",
                      })
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}

            {saveNetworkMutation.isSuccess ? (
              <Alert variant="success">
                <AlertTitle>{t("admin.saveNetworkSettingsSuccess")}</AlertTitle>
              </Alert>
            ) : null}

            {testNetworkMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.testNetworkSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {testNetworkMutation.error instanceof Error
                    ? translateApiError(testNetworkMutation.error, t, {
                        fallbackKey: "admin.testNetworkSettingsFailed",
                      })
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}

            {probeResult ? (
              <Alert variant={probeResult.ok ? "success" : "destructive"}>
                <AlertTitle>
                  {probeResult.ok
                    ? t("admin.testNetworkSettingsSuccess")
                    : t("admin.testNetworkSettingsFailed")}
                </AlertTitle>
                <AlertDescription>
                  {probeResult.ok
                    ? t("admin.testNetworkSettingsSuccessDetail", {
                        status: probeResult.statusCode ?? 0,
                        ms: probeResult.durationMs ?? 0,
                        via: probeResult.via ?? "direct",
                      })
                    : (probeResult.error ?? t("common.error"))}
                </AlertDescription>
              </Alert>
            ) : null}

            <FieldGroup>
              <Field data-invalid={Boolean(networkErrors.proxyUrl)}>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="proxyUrl">{t("admin.proxyUrl")}</FieldLabel>
                  <FieldHelp label={t("admin.proxyUrlHelp")}>
                    {t("admin.proxyUrlTooltip")}
                  </FieldHelp>
                </div>
                <Input
                  id="proxyUrl"
                  placeholder="socks5h://127.0.0.1:7890"
                  className="max-w-xl"
                  autoComplete="off"
                  aria-invalid={Boolean(networkErrors.proxyUrl)}
                  {...networkForm.register("proxyUrl")}
                />
                <FieldDescription>{t("admin.proxyUrlDescription")}</FieldDescription>
                <FieldError errors={[networkErrors.proxyUrl]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="proxyUser">{t("admin.proxyUser")}</FieldLabel>
                <Input
                  id="proxyUser"
                  className="max-w-md"
                  autoComplete="off"
                  {...networkForm.register("proxyUser")}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="proxyPassword">{t("admin.proxyPassword")}</FieldLabel>
                <Input
                  id="proxyPassword"
                  type="password"
                  className="max-w-md"
                  autoComplete="new-password"
                  placeholder={
                    hasProxyPassword
                      ? t("admin.proxyPasswordStored")
                      : t("admin.proxyPasswordEmpty")
                  }
                  {...networkForm.register("proxyPassword")}
                />
                <FieldDescription>{t("admin.proxyPasswordDescription")}</FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="noProxy">{t("admin.noProxy")}</FieldLabel>
                <Input
                  id="noProxy"
                  placeholder="localhost,127.0.0.1,.lan"
                  className="max-w-xl"
                  autoComplete="off"
                  {...networkForm.register("noProxy")}
                />
                <FieldDescription>{t("admin.noProxyDescription")}</FieldDescription>
              </Field>

              <Field className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  disabled={testNetworkMutation.isPending || saveNetworkMutation.isPending}
                  onClick={() => {
                    void networkForm.handleSubmit((values) => {
                      testNetworkMutation.reset();
                      saveNetworkMutation.reset();
                      setProbeResult(null);
                      testNetworkMutation.mutate(toNetworkBody(values));
                    })();
                  }}
                >
                  {testNetworkMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                  {testNetworkMutation.isPending
                    ? t("admin.testingNetwork")
                    : t("admin.testNetworkSettings")}
                </Button>
                <Button
                  type="submit"
                  disabled={saveNetworkMutation.isPending || testNetworkMutation.isPending}
                >
                  {saveNetworkMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                  {saveNetworkMutation.isPending
                    ? t("admin.saving")
                    : t("admin.saveNetworkSettings")}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CpuIcon className="size-5" />
            {t("admin.transcodeSettings")}
          </CardTitle>
          <CardDescription>{t("admin.transcodeSettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={transcodeForm.handleSubmit((values) => {
              saveTranscodeMutation.reset();
              const boost =
                values.downmixBoost.trim() === "" ? undefined : Number(values.downmixBoost);
              saveTranscodeMutation.mutate({
                hwAccel: values.hwAccel,
                downmixAlgorithm: values.downmixAlgorithm,
                downmixBoost: boost,
                toneMappingEnabled: values.toneMappingEnabled,
                toneMappingAlgorithm: values.toneMappingAlgorithm,
              });
            })}
          >
            {saveTranscodeMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.saveTranscodeSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {saveTranscodeMutation.error instanceof Error
                    ? translateApiError(saveTranscodeMutation.error, t, {
                        fallbackKey: "admin.saveTranscodeSettingsFailed",
                      })
                    : t("common.error")}
                </AlertDescription>
              </Alert>
            ) : null}

            {saveTranscodeMutation.isSuccess ? (
              <Alert variant="success">
                <AlertTitle>{t("admin.saveTranscodeSettingsSuccess")}</AlertTitle>
              </Alert>
            ) : null}

            <FieldGroup>
              <Field>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="hwAccel">{t("admin.hwAccel")}</FieldLabel>
                  <FieldHelp label={t("admin.hwAccelHelp")}>{t("admin.hwAccelTooltip")}</FieldHelp>
                </div>
                <Controller
                  control={transcodeForm.control}
                  name="hwAccel"
                  render={({ field }) => (
                    <NativeSelect
                      id="hwAccel"
                      className="w-full max-w-md"
                      value={field.value}
                      onChange={(event) => {
                        const value = event.target.value as HwAccelValue;
                        if (hwAccelOptions.includes(value)) {
                          field.onChange(value);
                        }
                      }}
                    >
                      <NativeSelectOption value="off">{t("admin.hwAccelOff")}</NativeSelectOption>
                      <NativeSelectOption value="qsv">{t("admin.hwAccelQsv")}</NativeSelectOption>
                      <NativeSelectOption value="vaapi">
                        {t("admin.hwAccelVaapi")}
                      </NativeSelectOption>
                      <NativeSelectOption value="nvenc">
                        {t("admin.hwAccelNvenc")}
                      </NativeSelectOption>
                      <NativeSelectOption value="rockchip">
                        {t("admin.hwAccelRockchip")}
                      </NativeSelectOption>
                    </NativeSelect>
                  )}
                />
                <FieldDescription>{t("admin.hwAccelDescription")}</FieldDescription>
              </Field>

              <Field>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="downmixAlgorithm">{t("admin.downmixAlgorithm")}</FieldLabel>
                  <FieldHelp label={t("admin.downmixAlgorithmHelp")}>
                    {t("admin.downmixAlgorithmTooltip")}
                  </FieldHelp>
                </div>
                <Controller
                  control={transcodeForm.control}
                  name="downmixAlgorithm"
                  render={({ field }) => (
                    <NativeSelect
                      id="downmixAlgorithm"
                      className="w-full max-w-md"
                      value={field.value}
                      onChange={(event) => {
                        field.onChange(event.target.value as DownmixValue);
                      }}
                    >
                      <NativeSelectOption value="ac4">{t("admin.downmixAc4")}</NativeSelectOption>
                      <NativeSelectOption value="none">{t("admin.downmixNone")}</NativeSelectOption>
                      <NativeSelectOption value="dave750">
                        {t("admin.downmixDave750")}
                      </NativeSelectOption>
                      <NativeSelectOption value="nightmodeDialogue">
                        {t("admin.downmixNightmodeDialogue")}
                      </NativeSelectOption>
                      <NativeSelectOption value="rfc7845">
                        {t("admin.downmixRfc7845")}
                      </NativeSelectOption>
                    </NativeSelect>
                  )}
                />
                <FieldDescription>{t("admin.downmixAlgorithmDescription")}</FieldDescription>
              </Field>

              <Field data-invalid={Boolean(transcodeErrors.downmixBoost)}>
                <FieldLabel htmlFor="downmixBoost">{t("admin.downmixBoost")}</FieldLabel>
                <Input
                  id="downmixBoost"
                  type="number"
                  min={0.5}
                  max={3}
                  step={0.1}
                  className="max-w-md"
                  aria-invalid={Boolean(transcodeErrors.downmixBoost)}
                  {...transcodeForm.register("downmixBoost")}
                />
                <FieldDescription>{t("admin.downmixBoostDescription")}</FieldDescription>
                <FieldError errors={[transcodeErrors.downmixBoost]} />
              </Field>

              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Controller
                    control={transcodeForm.control}
                    name="toneMappingEnabled"
                    render={({ field }) => (
                      <Switch
                        id="toneMappingEnabled"
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <FieldLabel htmlFor="toneMappingEnabled">
                    {t("admin.toneMappingEnabled")}
                  </FieldLabel>
                  <FieldHelp label={t("admin.toneMappingEnabledHelp")}>
                    {t("admin.toneMappingEnabledTooltip")}
                  </FieldHelp>
                </div>
              </Field>
              <FieldDescription>{t("admin.toneMappingEnabledDescription")}</FieldDescription>

              <Field data-disabled={!toneMappingEnabled || undefined}>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="toneMappingAlgorithm">
                    {t("admin.toneMappingAlgorithm")}
                  </FieldLabel>
                  <FieldHelp label={t("admin.toneMappingAlgorithmHelp")}>
                    {t("admin.toneMappingAlgorithmTooltip")}
                  </FieldHelp>
                </div>
                <Controller
                  control={transcodeForm.control}
                  name="toneMappingAlgorithm"
                  render={({ field }) => (
                    <NativeSelect
                      id="toneMappingAlgorithm"
                      className="w-full max-w-md"
                      value={field.value}
                      disabled={!toneMappingEnabled}
                      onChange={(event) => {
                        field.onChange(event.target.value as ToneAlgoValue);
                      }}
                    >
                      <NativeSelectOption value="bt2390">
                        {t("admin.toneAlgoBt2390")}
                      </NativeSelectOption>
                      <NativeSelectOption value="hable">
                        {t("admin.toneAlgoHable")}
                      </NativeSelectOption>
                      <NativeSelectOption value="reinhard">
                        {t("admin.toneAlgoReinhard")}
                      </NativeSelectOption>
                      <NativeSelectOption value="mobius">
                        {t("admin.toneAlgoMobius")}
                      </NativeSelectOption>
                    </NativeSelect>
                  )}
                />
                <FieldDescription>{t("admin.toneMappingAlgorithmDescription")}</FieldDescription>
              </Field>

              <Field>
                <Button type="submit" disabled={saveTranscodeMutation.isPending}>
                  {saveTranscodeMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                  {saveTranscodeMutation.isPending
                    ? t("admin.saving")
                    : t("admin.saveTranscodeSettings")}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <TvIcon className="size-5" />
            {t("admin.dlnaSettings")}
          </CardTitle>
          <CardDescription>{t("admin.dlnaSettingsDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          {dlnaQuery.isLoading ? <Skeleton className="h-32 w-full" /> : null}
          {saveDlnaMutation.isError ? (
            <Alert variant="destructive" className="mb-4">
              <AlertTitle>{t("admin.saveDLNASettingsFailed")}</AlertTitle>
              <AlertDescription>
                {translateApiError(saveDlnaMutation.error, t, {
                  fallbackKey: "admin.saveDLNASettingsFailed",
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          {saveDlnaMutation.isSuccess ? (
            <Alert variant="success" className="mb-4">
              <AlertTitle>{t("admin.saveDLNASettingsSuccess")}</AlertTitle>
            </Alert>
          ) : null}
          <form
            className="space-y-4"
            onSubmit={dlnaForm.handleSubmit((values) => {
              saveDlnaMutation.mutate({
                enabled: values.enabled,
                userId: values.userId,
              });
            })}
          >
            <FieldGroup>
              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Controller
                    control={dlnaForm.control}
                    name="enabled"
                    render={({ field }) => (
                      <Switch
                        id="dlnaEnabled"
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <FieldLabel htmlFor="dlnaEnabled">{t("admin.dlnaEnabled")}</FieldLabel>
                </div>
              </Field>

              <Field data-invalid={Boolean(dlnaErrors.userId)}>
                <FieldLabel htmlFor="dlnaUser">{t("admin.dlnaUser")}</FieldLabel>
                <Controller
                  control={dlnaForm.control}
                  name="userId"
                  render={({ field }) => (
                    <NativeSelect
                      id="dlnaUser"
                      className="w-full max-w-md"
                      value={field.value}
                      aria-invalid={Boolean(dlnaErrors.userId)}
                      onChange={(event) => {
                        field.onChange(event.target.value);
                      }}
                    >
                      <NativeSelectOption value="">{t("admin.dlnaUser")}</NativeSelectOption>
                      {tvUsers.map((user) => (
                        <NativeSelectOption key={user.id} value={user.id ?? ""}>
                          {user.email}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  )}
                />
                <FieldDescription>{t("admin.dlnaUserDescription")}</FieldDescription>
                <FieldError errors={[dlnaErrors.userId]} />
              </Field>

              <FieldDescription>{t("admin.dlnaPortHint")}</FieldDescription>

              <Field>
                <Button type="submit" disabled={saveDlnaMutation.isPending}>
                  {saveDlnaMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                  {saveDlnaMutation.isPending ? t("admin.saving") : t("admin.saveDLNASettings")}
                </Button>
              </Field>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Trash2Icon className="size-5" />
            {t("admin.trashRetentionTitle")}
          </CardTitle>
          <CardDescription>{t("admin.trashRetentionDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          {saveTrashMutation.isError ? (
            <Alert variant="destructive" className="mb-4">
              <AlertTitle>{t("admin.trashSettingsFailed")}</AlertTitle>
              <AlertDescription>
                {translateApiError(saveTrashMutation.error, t, {
                  fallbackKey: "admin.trashSettingsFailed",
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          {saveTrashMutation.isSuccess ? (
            <Alert variant="success" className="mb-4">
              <AlertTitle>{t("admin.saveTrashSettingsSuccess")}</AlertTitle>
            </Alert>
          ) : null}
          <TrashSettingsForm
            enabled={Boolean(trashSettingsQuery.data?.enabled)}
            retentionDays={trashSettingsQuery.data?.retentionDays ?? 1}
            pending={saveTrashMutation.isPending}
            onSave={(values) => {
              saveTrashMutation.mutate(values);
            }}
          />
        </CardContent>
      </Card>
    </div>
  );
}
