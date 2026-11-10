import { ArrowRight, Target } from "lucide-react";
import { api } from "../../api/client";
import type { Strategy } from "../../api/types";
import { EditableText } from "../../components/EditableText";
import { GenerationPanel } from "../../components/GenerationPanel";
import { Button, ErrorState, Field, TagList } from "../../components/ui";
import { useGeneration } from "../../hooks/useGeneration";
import { formatDuration, t } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

export function StrategyStep({ channel, goTo }: StepProps) {
  const gen = useGeneration(channel.id, "strategy");
  const save = useChannelUpdate(channel.id, (s: Strategy) => api.putStrategy(channel.id, s));
  const advance = useChannelUpdate(channel.id, () => api.changeStatus(channel.id, "branding"));
  const s = channel.strategy;
  const locked = channel.status === "archived";

  const edit = (patch: Partial<Strategy>) => (s ? save.mutateAsync({ ...s, ...patch }) : Promise.resolve());

  const next = () => {
    if (channel.status === "draft") advance.mutate(undefined, { onSuccess: () => goTo("branding") });
    else goTo("branding");
  };

  return (
    <GenerationPanel
      title={t("strategy.title")}
      description={t("strategy.description")}
      gen={gen}
      provenance={s?.provenance}
      disabled={gen.running || locked}
      emptyIcon={Target}
      emptyText={t("strategy.empty")}
    >
      {s && (
        <>
          <dl className="fields">
            <EditableText label={t("strategy.audience")} value={s.targetAudience} multiline onSave={(v) => edit({ targetAudience: v })} />
            <EditableText label={t("strategy.geography")} value={s.audienceGeography} onSave={(v) => edit({ audienceGeography: v })} />
            <EditableText label={t("strategy.positioning")} value={s.positioning} multiline onSave={(v) => edit({ positioning: v })} />
            <EditableText label={t("strategy.editorial")} value={s.editorialIdentity} multiline onSave={(v) => edit({ editorialIdentity: v })} />
            <Field label={t("strategy.pillars")}>
              <TagList items={s.contentPillars} />
            </Field>
            <Field label={t("strategy.formats")}>
              <ul className="plain-list">
                {s.videoFormats.map((f) => (
                  <li key={f.name}>
                    <strong>{f.name}</strong>{" "}
                    <span className="small muted tabular">
                      · {f.aspectRatio} · {formatDuration(f.durationSeconds)}
                    </span>
                    <p className="small muted">{f.description}</p>
                  </li>
                ))}
              </ul>
            </Field>
            <Field label={t("strategy.schedule")}>
              <span className="tabular">
                {t("strategy.scheduleValue", {
                  count: s.publishingFrequency.videosPerWeek,
                  days: s.publishingFrequency.days.join(", "),
                  time: s.publishingFrequency.localTime,
                  tz: s.publishingFrequency.timeZone,
                })}
              </span>
            </Field>
            <Field label={t("strategy.competitors")}>
              <TagList items={s.competitorReferences} />
              <p className="small muted">{s.competitorNote}</p>
            </Field>
            <Field label={t("strategy.monetization")}>
              <TagList items={s.monetizationOptions} />
            </Field>
          </dl>
          {advance.isError && <ErrorState error={advance.error} />}
          <div className="row">
            <Button variant="primary" icon={ArrowRight} loading={advance.isPending} disabled={locked} onClick={next}>
              {t("strategy.toBranding")}
            </Button>
          </div>
        </>
      )}
    </GenerationPanel>
  );
}
