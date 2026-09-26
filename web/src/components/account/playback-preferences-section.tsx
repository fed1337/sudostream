import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { extractApiErrorMessage } from "@/lib/api-error";
import { getApiMePlaybackPreferencesOptions } from "@/client/@tanstack/react-query.gen";
import { patchApiMePlaybackPreferences } from "@/client/sdk.gen";

const MAX_LANGS = 3;

export function PlaybackPreferencesSection() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const prefsQuery = useQuery({
    ...getApiMePlaybackPreferencesOptions(),
  });

  const saved = prefsQuery.data?.audioLanguages ?? [];
  const [lang1, setLang1] = useState("");
  const [lang2, setLang2] = useState("");
  const [lang3, setLang3] = useState("");
  const seedKey = saved.join("|");
  const display1 = lang1 || saved[0] || "";
  const display2 = lang2 || saved[1] || "";
  const display3 = lang3 || saved[2] || "";

  const saveMutation = useMutation({
    mutationFn: async () => {
      const audioLanguages = [display1, display2, display3]
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
    onSuccess: () => {
      setLang1("");
      setLang2("");
      setLang3("");
      void queryClient.invalidateQueries({
        queryKey: getApiMePlaybackPreferencesOptions().queryKey,
      });
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{t("account.playbackPreferences")}</CardTitle>
        <CardDescription>{t("account.playbackPreferencesDescription")}</CardDescription>
      </CardHeader>
      <CardContent key={seedKey}>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            saveMutation.reset();
            saveMutation.mutate();
          }}
        >
          {saveMutation.isError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("account.playbackPreferencesSaveFailed")}</AlertTitle>
              <AlertDescription>
                {saveMutation.error instanceof Error
                  ? saveMutation.error.message
                  : t("common.error")}
              </AlertDescription>
            </Alert>
          ) : null}
          {saveMutation.isSuccess ? (
            <Alert variant="success">
              <AlertTitle>{t("account.playbackPreferencesSaveSuccess")}</AlertTitle>
            </Alert>
          ) : null}
          <FieldGroup>
            {[
              { id: "audio-lang-1", value: display1, set: setLang1 },
              { id: "audio-lang-2", value: display2, set: setLang2 },
              { id: "audio-lang-3", value: display3, set: setLang3 },
            ].map((field, index) => (
              <Field key={field.id}>
                <FieldLabel htmlFor={field.id}>
                  {t("account.audioLanguagePriority", { rank: index + 1 })}
                </FieldLabel>
                <Input
                  id={field.id}
                  value={field.value}
                  placeholder={t("account.audioLanguagePlaceholder")}
                  maxLength={16}
                  onChange={(event) => field.set(event.target.value)}
                />
              </Field>
            ))}
            <FieldDescription>
              {t("account.playbackPreferencesHint", { max: MAX_LANGS })}
            </FieldDescription>
          </FieldGroup>
          <Button type="submit" disabled={saveMutation.isPending || prefsQuery.isLoading}>
            {t("common.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
