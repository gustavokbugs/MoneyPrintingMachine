import { ArrowRight } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { ApiError, api } from "../../api/client";
import { Button, ErrorState, Section } from "../../components/ui";
import { t } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

type FieldName = "niche" | "language" | "geoTarget";

export function NicheStep({ channel, goTo }: StepProps) {
  const [niche, setNiche] = useState(channel.niche);
  const [language, setLanguage] = useState(channel.language);
  const [geo, setGeo] = useState(channel.geoTarget);
  const refs = { niche: useRef<HTMLInputElement>(null), language: useRef<HTMLInputElement>(null), geoTarget: useRef<HTMLInputElement>(null) };
  const update = useChannelUpdate(channel.id, (input: { niche: string; language: string; geoTarget: string }) => api.updateChannel(channel.id, input));
  const locked = channel.status === "archived";

  const err = update.error instanceof ApiError ? update.error : null;
  const fieldError = (name: FieldName) => (err?.field === name ? err.message : null);
  const generalError = update.isError && !(err?.field && err.field in refs) ? update.error : null;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate(
      { niche, language, geoTarget: geo },
      {
        onSuccess: () => goTo("strategy"),
        onError: (error) => {
          // Move focus to the invalid field so keyboard and screen reader users land on the problem.
          if (error instanceof ApiError && error.field && error.field in refs) refs[error.field as FieldName].current?.focus();
        },
      },
    );
  };

  const describedBy = (name: FieldName, helpId?: string) => [helpId, fieldError(name) ? `${name}-error` : ""].filter(Boolean).join(" ") || undefined;

  return (
    <Section title={t("niche.title")} description={t("niche.help")}>
      <form className="form" onSubmit={submit} noValidate>
        <div className="field-group">
          <label htmlFor="niche">
            {t("niche.label")} <span aria-hidden="true">*</span>
          </label>
          <input
            ref={refs.niche}
            id="niche"
            required
            aria-required="true"
            maxLength={120}
            value={niche}
            disabled={locked}
            aria-invalid={fieldError("niche") ? true : undefined}
            aria-describedby={describedBy("niche")}
            onChange={(e) => setNiche(e.target.value)}
          />
          {fieldError("niche") && (
            <span id="niche-error" className="error-text">
              {fieldError("niche")}
            </span>
          )}
        </div>
        <div className="grid-halves">
          <div className="field-group">
            <label htmlFor="language">{t("niche.language")}</label>
            <input
              ref={refs.language}
              id="language"
              value={language}
              disabled={locked}
              aria-invalid={fieldError("language") ? true : undefined}
              aria-describedby={describedBy("language", "language-help")}
              onChange={(e) => setLanguage(e.target.value)}
            />
            <span id="language-help" className="helper">
              {t("niche.languageHelp")}
            </span>
            {fieldError("language") && (
              <span id="language-error" className="error-text">
                {fieldError("language")}
              </span>
            )}
          </div>
          <div className="field-group">
            <label htmlFor="geo">{t("niche.geo")}</label>
            <input
              ref={refs.geoTarget}
              id="geo"
              maxLength={2}
              autoComplete="country"
              value={geo}
              disabled={locked}
              aria-invalid={fieldError("geoTarget") ? true : undefined}
              aria-describedby={describedBy("geoTarget", "geo-help")}
              onChange={(e) => setGeo(e.target.value.toUpperCase())}
            />
            <span id="geo-help" className="helper">
              {t("niche.geoHelp")}
            </span>
            {fieldError("geoTarget") && (
              <span id="geoTarget-error" className="error-text">
                {fieldError("geoTarget")}
              </span>
            )}
          </div>
        </div>
        {generalError && <ErrorState error={generalError} />}
        <div className="row">
          <Button type="submit" variant="primary" icon={ArrowRight} loading={update.isPending} disabled={locked || !niche.trim()}>
            {t("common.continue")}
          </Button>
        </div>
      </form>
    </Section>
  );
}
