import { api } from "../../api/client";
import type { Strategy } from "../../api/types";
import { EditableText } from "../../components/EditableText";
import { Empty, ErrorState, Field, JobProgress, ProvenanceNote, Section, TagList } from "../../components/ui";
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
    if (channel.status === "draft") {
      advance.mutate(undefined, { onSuccess: () => goTo("branding") });
    } else {
      goTo("branding");
    }
  };

  return (
    <Section
      title={t("strategy.title")}
      actions={
        <button type="button" className="btn btn-secondary" disabled={gen.running || locked} onClick={gen.start}>
          {s ? t("common.regenerate") : t("common.generate")}
        </button>
      }
    >
      {gen.startError && <ErrorState error={gen.startError} />}
      {gen.jobId && <JobProgress job={gen.job.data} error={gen.job.error} />}
      {!s && !gen.jobId && <Empty>{t("strategy.empty")}</Empty>}
      {s && (
        <>
          <ProvenanceNote p={s.provenance} />
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
                    <strong>{f.name}</strong> · {f.aspectRatio} · {formatDuration(f.durationSeconds)}
                    <p className="small muted">{f.description}</p>
                  </li>
                ))}
              </ul>
            </Field>
            <Field label={t("strategy.schedule")}>
              {t("strategy.scheduleValue", {
                count: s.publishingFrequency.videosPerWeek,
                days: s.publishingFrequency.days.join(", "),
                time: s.publishingFrequency.localTime,
                tz: s.publishingFrequency.timeZone,
              })}
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
            <button type="button" className="btn btn-primary" disabled={advance.isPending || locked} onClick={next}>
              {t("strategy.toBranding")}
            </button>
          </div>
        </>
      )}
    </Section>
  );
}
