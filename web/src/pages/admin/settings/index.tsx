import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { CpuIcon, CircleHelpIcon, NetworkIcon, SettingsIcon, TvIcon } from "lucide-react";

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
  getApiAdminUsersOptions,
} from "@/client/@tanstack/react-query.gen";
import {
  patchApiAdminDlnaSettings,
  patchApiAdminNetworkSettings,
  patchApiAdminSettings,
  patchApiAdminTranscodeSettings,
  postApiAdminNetworkSettingsTest,
} from "@/client/sdk.gen";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { extractApiErrorMessage } from "@/lib/api-error";

type Settings = SudoStreamInternalAuthSettings;
type TranscodeSettings = SudoStreamInternalTranscodeTranscodeSettings;
type NetworkSettings = SudoStreamInternalNetworkSettings;
type DlnaSettings = SudoStreamInternalDlnaSettings;

type HwAccelValue = NonNullable<TranscodeSettings["hwAccel"]>;
type DownmixValue = NonNullable<TranscodeSettings["downmixAlgorithm"]>;
type ToneAlgoValue = NonNullable<TranscodeSettings["toneMappingAlgorithm"]>;

const hwAccelOptions: HwAccelValue[] = ["off", "qsv", "vaapi", "nvenc", "rockchip"];
const downmixOptions: DownmixValue[] = ["none", "ac4", "dave750", "nightmodeDialogue", "rfc7845"];
const toneAlgoOptions: ToneAlgoValue[] = ["bt2390", "hable", "reinhard", "mobius"];

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
  const usersQuery = useQuery({
    ...getApiAdminUsersOptions(),
  });

  const form = useForm<Settings>({
    defaultValues: {},
  });

  const [hwAccel, setHwAccel] = useState<HwAccelValue>("off");
  const [downmixAlgorithm, setDownmixAlgorithm] = useState<DownmixValue>("ac4");
  const [downmixBoost, setDownmixBoost] = useState("1");
  const [toneMappingEnabled, setToneMappingEnabled] = useState(true);
  const [toneMappingAlgorithm, setToneMappingAlgorithm] = useState<ToneAlgoValue>("bt2390");
  const [prevTranscode, setPrevTranscode] = useState(transcodeQuery.data);

  const [proxyUrl, setProxyUrl] = useState("");
  const [proxyUser, setProxyUser] = useState("");
  const [proxyPassword, setProxyPassword] = useState("");
  const [noProxy, setNoProxy] = useState("");
  const [hasProxyPassword, setHasProxyPassword] = useState(false);
  const [prevNetwork, setPrevNetwork] = useState(networkQuery.data);
  const [probeResult, setProbeResult] = useState<SudoStreamInternalNetworkProbeResult | null>(null);

  const [dlnaEnabled, setDlnaEnabled] = useState(false);
  const [dlnaUserId, setDlnaUserId] = useState("");
  const [prevDlna, setPrevDlna] = useState(dlnaQuery.data);

  if (transcodeQuery.data !== prevTranscode) {
    setPrevTranscode(transcodeQuery.data);
    if (transcodeQuery.data?.hwAccel) {
      setHwAccel(transcodeQuery.data.hwAccel);
    }
    if (transcodeQuery.data?.downmixAlgorithm) {
      setDownmixAlgorithm(transcodeQuery.data.downmixAlgorithm);
    }
    if (typeof transcodeQuery.data?.downmixBoost === "number") {
      setDownmixBoost(String(transcodeQuery.data.downmixBoost));
    }
    if (typeof transcodeQuery.data?.toneMappingEnabled === "boolean") {
      setToneMappingEnabled(transcodeQuery.data.toneMappingEnabled);
    }
    if (transcodeQuery.data?.toneMappingAlgorithm) {
      setToneMappingAlgorithm(transcodeQuery.data.toneMappingAlgorithm);
    }
  }

  if (networkQuery.data !== prevNetwork) {
    setPrevNetwork(networkQuery.data);
    if (networkQuery.data) {
      setProxyUrl(networkQuery.data.proxyUrl ?? "");
      setProxyUser(networkQuery.data.proxyUser ?? "");
      setProxyPassword("");
      setNoProxy(networkQuery.data.noProxy ?? "");
      setHasProxyPassword(Boolean(networkQuery.data.hasProxyPassword));
    }
  }

  if (dlnaQuery.data !== prevDlna) {
    setPrevDlna(dlnaQuery.data);
    if (dlnaQuery.data) {
      setDlnaEnabled(Boolean(dlnaQuery.data.enabled));
      setDlnaUserId(dlnaQuery.data.userId ?? "");
    }
  }

  useEffect(() => {
    if (settingsQuery.data) {
      form.reset(settingsQuery.data);
    }
  }, [form, settingsQuery.data]);

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
      if (data.hwAccel) {
        setHwAccel(data.hwAccel);
      }
      if (data.downmixAlgorithm) {
        setDownmixAlgorithm(data.downmixAlgorithm);
      }
      if (typeof data.downmixBoost === "number") {
        setDownmixBoost(String(data.downmixBoost));
      }
      if (typeof data.toneMappingEnabled === "boolean") {
        setToneMappingEnabled(data.toneMappingEnabled);
      }
      if (data.toneMappingAlgorithm) {
        setToneMappingAlgorithm(data.toneMappingAlgorithm);
      }
    },
  });

  const networkDraft = (): NetworkSettings => ({
    proxyUrl,
    proxyUser: proxyUser || undefined,
    proxyPassword: proxyPassword || undefined,
    noProxy,
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
      setProxyUrl(data.proxyUrl ?? "");
      setProxyUser(data.proxyUser ?? "");
      setProxyPassword("");
      setNoProxy(data.noProxy ?? "");
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
      setDlnaEnabled(Boolean(data.enabled));
      setDlnaUserId(data.userId ?? "");
      void dlnaQuery.refetch();
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

  if (settingsQuery.isLoading || transcodeQuery.isLoading || networkQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-96 w-full rounded-xl" />
      </div>
    );
  }

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
                    ? saveMutation.error.message
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
            onSubmit={(event) => {
              event.preventDefault();
              saveNetworkMutation.reset();
              testNetworkMutation.reset();
              setProbeResult(null);
              saveNetworkMutation.mutate(networkDraft());
            }}
          >
            {saveNetworkMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.saveNetworkSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {saveNetworkMutation.error instanceof Error
                    ? saveNetworkMutation.error.message
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
                    ? testNetworkMutation.error.message
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
              <Field>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="proxyUrl">{t("admin.proxyUrl")}</FieldLabel>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("admin.proxyUrlHelp")}
                      >
                        <CircleHelpIcon className="size-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-sm whitespace-pre-line" sideOffset={6}>
                      {t("admin.proxyUrlTooltip")}
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  id="proxyUrl"
                  value={proxyUrl}
                  onChange={(event) => setProxyUrl(event.target.value)}
                  placeholder="socks5h://127.0.0.1:7890"
                  className="max-w-xl"
                  autoComplete="off"
                />
                <FieldDescription>{t("admin.proxyUrlDescription")}</FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="proxyUser">{t("admin.proxyUser")}</FieldLabel>
                <Input
                  id="proxyUser"
                  value={proxyUser}
                  onChange={(event) => setProxyUser(event.target.value)}
                  className="max-w-md"
                  autoComplete="off"
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="proxyPassword">{t("admin.proxyPassword")}</FieldLabel>
                <Input
                  id="proxyPassword"
                  type="password"
                  value={proxyPassword}
                  onChange={(event) => setProxyPassword(event.target.value)}
                  className="max-w-md"
                  autoComplete="new-password"
                  placeholder={
                    hasProxyPassword
                      ? t("admin.proxyPasswordStored")
                      : t("admin.proxyPasswordEmpty")
                  }
                />
                <FieldDescription>{t("admin.proxyPasswordDescription")}</FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="noProxy">{t("admin.noProxy")}</FieldLabel>
                <Input
                  id="noProxy"
                  value={noProxy}
                  onChange={(event) => setNoProxy(event.target.value)}
                  placeholder="localhost,127.0.0.1,.lan"
                  className="max-w-xl"
                  autoComplete="off"
                />
                <FieldDescription>{t("admin.noProxyDescription")}</FieldDescription>
              </Field>

              <Field className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  disabled={testNetworkMutation.isPending || saveNetworkMutation.isPending}
                  onClick={() => {
                    testNetworkMutation.reset();
                    saveNetworkMutation.reset();
                    setProbeResult(null);
                    testNetworkMutation.mutate(networkDraft());
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
            onSubmit={(event) => {
              event.preventDefault();
              saveTranscodeMutation.reset();
              const boost = Number.parseFloat(downmixBoost);
              saveTranscodeMutation.mutate({
                hwAccel,
                downmixAlgorithm,
                downmixBoost: Number.isFinite(boost) ? boost : undefined,
                toneMappingEnabled,
                toneMappingAlgorithm,
              });
            }}
          >
            {saveTranscodeMutation.isError ? (
              <Alert variant="destructive">
                <AlertTitle>{t("admin.saveTranscodeSettingsFailed")}</AlertTitle>
                <AlertDescription>
                  {saveTranscodeMutation.error instanceof Error
                    ? saveTranscodeMutation.error.message
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
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("admin.hwAccelHelp")}
                      >
                        <CircleHelpIcon className="size-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs whitespace-pre-line" sideOffset={6}>
                      {t("admin.hwAccelTooltip")}
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Select
                  value={hwAccel}
                  onValueChange={(value) => {
                    if (hwAccelOptions.includes(value as HwAccelValue)) {
                      setHwAccel(value as HwAccelValue);
                    }
                  }}
                >
                  <SelectTrigger id="hwAccel" className="w-full max-w-md">
                    <SelectValue placeholder={t("admin.hwAccel")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="off">{t("admin.hwAccelOff")}</SelectItem>
                    <SelectItem value="qsv">{t("admin.hwAccelQsv")}</SelectItem>
                    <SelectItem value="vaapi">{t("admin.hwAccelVaapi")}</SelectItem>
                    <SelectItem value="nvenc">{t("admin.hwAccelNvenc")}</SelectItem>
                    <SelectItem value="rockchip">{t("admin.hwAccelRockchip")}</SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>{t("admin.hwAccelDescription")}</FieldDescription>
              </Field>

              <Field>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="downmixAlgorithm">{t("admin.downmixAlgorithm")}</FieldLabel>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("admin.downmixAlgorithmHelp")}
                      >
                        <CircleHelpIcon className="size-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs whitespace-pre-line" sideOffset={6}>
                      {t("admin.downmixAlgorithmTooltip")}
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Select
                  value={downmixAlgorithm}
                  onValueChange={(value) => {
                    if (downmixOptions.includes(value as DownmixValue)) {
                      setDownmixAlgorithm(value as DownmixValue);
                    }
                  }}
                >
                  <SelectTrigger id="downmixAlgorithm" className="w-full max-w-md">
                    <SelectValue placeholder={t("admin.downmixAlgorithm")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ac4">{t("admin.downmixAc4")}</SelectItem>
                    <SelectItem value="none">{t("admin.downmixNone")}</SelectItem>
                    <SelectItem value="dave750">{t("admin.downmixDave750")}</SelectItem>
                    <SelectItem value="nightmodeDialogue">
                      {t("admin.downmixNightmodeDialogue")}
                    </SelectItem>
                    <SelectItem value="rfc7845">{t("admin.downmixRfc7845")}</SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>{t("admin.downmixAlgorithmDescription")}</FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="downmixBoost">{t("admin.downmixBoost")}</FieldLabel>
                <Input
                  id="downmixBoost"
                  type="number"
                  min={0.5}
                  max={3}
                  step={0.1}
                  className="max-w-md"
                  value={downmixBoost}
                  onChange={(event) => setDownmixBoost(event.target.value)}
                />
                <FieldDescription>{t("admin.downmixBoostDescription")}</FieldDescription>
              </Field>

              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Switch
                    id="toneMappingEnabled"
                    checked={toneMappingEnabled}
                    onCheckedChange={setToneMappingEnabled}
                  />
                  <FieldLabel htmlFor="toneMappingEnabled">
                    {t("admin.toneMappingEnabled")}
                  </FieldLabel>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("admin.toneMappingEnabledHelp")}
                      >
                        <CircleHelpIcon className="size-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs whitespace-pre-line" sideOffset={6}>
                      {t("admin.toneMappingEnabledTooltip")}
                    </TooltipContent>
                  </Tooltip>
                </div>
              </Field>
              <FieldDescription>{t("admin.toneMappingEnabledDescription")}</FieldDescription>

              <Field data-disabled={!toneMappingEnabled || undefined}>
                <div className="flex items-center gap-2">
                  <FieldLabel htmlFor="toneMappingAlgorithm">
                    {t("admin.toneMappingAlgorithm")}
                  </FieldLabel>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("admin.toneMappingAlgorithmHelp")}
                      >
                        <CircleHelpIcon className="size-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs whitespace-pre-line" sideOffset={6}>
                      {t("admin.toneMappingAlgorithmTooltip")}
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Select
                  value={toneMappingAlgorithm}
                  disabled={!toneMappingEnabled}
                  onValueChange={(value) => {
                    if (toneAlgoOptions.includes(value as ToneAlgoValue)) {
                      setToneMappingAlgorithm(value as ToneAlgoValue);
                    }
                  }}
                >
                  <SelectTrigger id="toneMappingAlgorithm" className="w-full max-w-md">
                    <SelectValue placeholder={t("admin.toneMappingAlgorithm")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="bt2390">{t("admin.toneAlgoBt2390")}</SelectItem>
                    <SelectItem value="hable">{t("admin.toneAlgoHable")}</SelectItem>
                    <SelectItem value="reinhard">{t("admin.toneAlgoReinhard")}</SelectItem>
                    <SelectItem value="mobius">{t("admin.toneAlgoMobius")}</SelectItem>
                  </SelectContent>
                </Select>
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
              <AlertDescription>{saveDlnaMutation.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {saveDlnaMutation.isSuccess ? (
            <Alert variant="success" className="mb-4">
              <AlertTitle>{t("admin.saveDLNASettingsSuccess")}</AlertTitle>
            </Alert>
          ) : null}
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              saveDlnaMutation.mutate({
                enabled: dlnaEnabled,
                userId: dlnaUserId,
              });
            }}
          >
            <FieldGroup>
              <Field orientation="horizontal">
                <div className="flex items-center gap-2">
                  <Switch id="dlnaEnabled" checked={dlnaEnabled} onCheckedChange={setDlnaEnabled} />
                  <FieldLabel htmlFor="dlnaEnabled">{t("admin.dlnaEnabled")}</FieldLabel>
                </div>
              </Field>

              <Field>
                <FieldLabel htmlFor="dlnaUser">{t("admin.dlnaUser")}</FieldLabel>
                <Select value={dlnaUserId || undefined} onValueChange={setDlnaUserId}>
                  <SelectTrigger id="dlnaUser" className="w-full max-w-md">
                    <SelectValue placeholder={t("admin.dlnaUser")} />
                  </SelectTrigger>
                  <SelectContent>
                    {tvUsers.map((user) => (
                      <SelectItem key={user.id} value={user.id ?? ""}>
                        {user.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldDescription>{t("admin.dlnaUserDescription")}</FieldDescription>
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
    </div>
  );
}
