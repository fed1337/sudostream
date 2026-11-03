import { useEffect, useMemo } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { extractApiErrorMessage, translateApiError } from "@/lib/api-error";
import { getApiMePlaybackPreferencesOptions } from "@/client/@tanstack/react-query.gen";
import { patchApiMePlaybackPreferences } from "@/client/sdk.gen";

const MAX_LANGS = 3;

export function PlaybackPreferencesSection() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const prefsQuery = useQuery({
    ...getApiMePlaybackPreferencesOptions(),
  });

  const schema = useMemo(
    () =>
      z.object({
        lang1: z.string().max(16),
        lang2: z.string().max(16),
        lang3: z.string().max(16),
      }),
    [],
  );
  type Values = z.infer<typeof schema>;

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { lang1: "", lang2: "", lang3: "" },
  });

  useEffect(() => {
    const langs = prefsQuery.data?.audioLanguages;
    if (!langs) {
      return;
    }
    form.reset({
      lang1: langs[0] ?? "",
      lang2: langs[1] ?? "",
      lang3: langs[2] ?? "",
    });
  }, [prefsQuery.data, form]);

  const saveMutation = useMutation({
    mutationFn: async (values: Values) => {
      const audioLanguages = [values.lang1, values.lang2, values.lang3]
        .map((value) => value.trim())
        .filter((value) => value.length > 0);
      const response = await patchApiMePlaybackPreferences({
        body: { audioLanguages },
      });
      if (response.error || !response.data) {
        throw new Error(
          extractApiErrorMessage(response.error) ?? t("account.playbackPreferencesSaveFailed"),
        );
      }
      return response.data;
    },
    onSuccess: (data) => {
      const langs = data.audioLanguages ?? [];
      form.reset({
        lang1: langs[0] ?? "",
        lang2: langs[1] ?? "",
        lang3: langs[2] ?? "",
      });
      void queryClient.invalidateQueries({
        queryKey: getApiMePlaybackPreferencesOptions().queryKey,
      });
    },
  });

  const errors = form.formState.errors;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{t("account.playbackPreferences")}</CardTitle>
        <CardDescription>{t("account.playbackPreferencesDescription")}</CardDescription>
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
              <AlertTitle>{t("account.playbackPreferencesSaveFailed")}</AlertTitle>
              <AlertDescription>
                {translateApiError(saveMutation.error, t, {
                  fallbackKey: "account.playbackPreferencesSaveFailed",
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          {saveMutation.isSuccess ? (
            <Alert variant="success">
              <AlertTitle>{t("account.playbackPreferencesSaveSuccess")}</AlertTitle>
            </Alert>
          ) : null}
          <FieldGroup>
            {(
              [
                { id: "audio-lang-1", name: "lang1" as const },
                { id: "audio-lang-2", name: "lang2" as const },
                { id: "audio-lang-3", name: "lang3" as const },
              ] as const
            ).map((field, index) => (
              <Field key={field.id} data-invalid={Boolean(errors[field.name])}>
                <FieldLabel htmlFor={field.id}>
                  {t("account.audioLanguagePriority", { rank: index + 1 })}
                </FieldLabel>
                <Input
                  id={field.id}
                  placeholder={t("account.audioLanguagePlaceholder")}
                  maxLength={16}
                  aria-invalid={Boolean(errors[field.name])}
                  {...form.register(field.name)}
                />
                <FieldError errors={[errors[field.name]]} />
              </Field>
            ))}
            <FieldDescription>
              {t("account.playbackPreferencesHint", { max: MAX_LANGS })}
            </FieldDescription>
          </FieldGroup>
          <Button type="submit" disabled={saveMutation.isPending || prefsQuery.isLoading}>
            {saveMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
            {t("common.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
