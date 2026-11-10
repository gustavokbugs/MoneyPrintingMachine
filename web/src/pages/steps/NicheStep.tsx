import { useState, type FormEvent } from "react";
import { api } from "../../api/client";
import { ErrorState, Section } from "../../components/ui";
import { t } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

export function NicheStep({ channel, goTo }: StepProps) {
  const [niche, setNiche] = useState(channel.niche);
  const [language, setLanguage] = useState(channel.language);
  const [geo, setGeo] = useState(channel.geoTarget);
  const update = useChannelUpdate(channel.id, (input: { niche: string; language: string; geoTarget: string }) =>
    api.updateChannel(channel.id, input),
  );
  const locked = channel.status === "archived";

  const submit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate({ niche, language, geoTarget: geo }, { onSuccess: () => goTo("strategy") });
  };

  return (
    <Section title={t("niche.title")}>
      <form className="form" onSubmit={submit}>
        <label htmlFor="niche">{t("niche.label")}</label>
        <input id="niche" required maxLength={120} value={niche} disabled={locked} onChange={(e) => setNiche(e.target.value)} />
        <div className="grid-2">
          <div>
            <label htmlFor="language">{t("niche.language")}</label>
            <input id="language" required value={language} disabled={locked} onChange={(e) => setLanguage(e.target.value)} />
          </div>
          <div>
            <label htmlFor="geo">{t("niche.geo")}</label>
            <input id="geo" required maxLength={2} value={geo} disabled={locked} onChange={(e) => setGeo(e.target.value.toUpperCase())} />
          </div>
        </div>
        <p className="small muted">{t("niche.help")}</p>
        {update.isError && <ErrorState error={update.error} />}
        <div className="row">
          <button type="submit" className="btn btn-primary" disabled={locked || update.isPending || !niche.trim()}>
            {update.isPending ? t("common.saving") : t("common.continue")}
          </button>
        </div>
      </form>
    </Section>
  );
}
