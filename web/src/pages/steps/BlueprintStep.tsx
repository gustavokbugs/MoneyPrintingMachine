import { Empty, ErrorState, JobProgress, ProvenanceNote, Section, TagList } from "../../components/ui";
import { useGeneration } from "../../hooks/useGeneration";
import { formatCalendarDate, formatDuration, t } from "../../i18n";
import type { StepProps } from "../ChannelWizard";

export function BlueprintStep({ channel, goTo }: StepProps) {
  const gen = useGeneration(channel.id, "blueprint");
  const bp = channel.blueprint;
  const locked = channel.status === "archived";

  return (
    <div className="stack">
      <Section
        title={t("blueprint.title")}
        actions={
          <button type="button" className="btn btn-secondary" disabled={gen.running || locked || !channel.strategy} onClick={gen.start}>
            {bp ? t("common.regenerate") : t("common.generate")}
          </button>
        }
      >
        <p className="small muted">{t("blueprint.slow")}</p>
        {gen.startError && <ErrorState error={gen.startError} />}
        {gen.jobId && <JobProgress job={gen.job.data} error={gen.job.error} />}
        {!bp && !gen.jobId && <Empty>{t("blueprint.empty")}</Empty>}
        {bp && <ProvenanceNote p={bp.provenance} />}
      </Section>

      {bp && (
        <>
          <div className="grid-2">
            <Section title={t("blueprint.priority")}>
              <ol>
                {bp.priorityTopics.map((p) => (
                  <li key={p}>{p}</li>
                ))}
              </ol>
            </Section>
            <Section title={t("blueprint.series")}>
              <ul className="plain-list">
                {bp.series.map((s) => (
                  <li key={s.name}>
                    <strong>{s.name}</strong> <span className="small muted">· {s.cadence}</span>
                    <p className="small">{s.description}</p>
                  </li>
                ))}
              </ul>
            </Section>
            <Section title={t("blueprint.formats")}>
              <ul className="plain-list">
                {bp.formats.map((f) => (
                  <li key={f.name}>
                    <strong>{f.name}</strong> · {f.aspectRatio} · {formatDuration(f.durationSeconds)}
                    <p className="small muted">{f.description}</p>
                  </li>
                ))}
              </ul>
            </Section>
            <Section title={t("blueprint.hooks")}>
              <TagList items={bp.hooks} />
            </Section>
          </div>

          <Section title={t("blueprint.ideas", { count: bp.ideas.length })}>
            <ol className="ideas">
              {bp.ideas.map((idea, i) => (
                <li key={`${i}-${idea.workingTitle}`}>
                  <details>
                    <summary>
                      <strong>{idea.workingTitle}</strong>{" "}
                      <span className="small muted">
                        · {idea.format} · {formatDuration(idea.suggestedDurationSeconds)}
                      </span>
                    </summary>
                    <dl className="fields">
                      <div className="field">
                        <dt>{t("blueprint.angle")}</dt>
                        <dd>{idea.angle}</dd>
                      </div>
                      <div className="field">
                        <dt>{t("blueprint.why")}</dt>
                        <dd>{idea.whyInteresting}</dd>
                      </div>
                      <div className="field">
                        <dt>{t("blueprint.sources")}</dt>
                        <dd>
                          <TagList items={idea.researchSources} />
                        </dd>
                      </div>
                      <div className="field">
                        <dt>{t("blueprint.originality")}</dt>
                        <dd>{idea.originalityNotes}</dd>
                      </div>
                    </dl>
                  </details>
                </li>
              ))}
            </ol>
          </Section>

          <Section title={t("blueprint.calendar")}>
            <div className="table-wrap">
              <table>
                <tbody>
                  {bp.calendar.map((e) => (
                    <tr key={`${e.date}-${e.ideaIndex}`}>
                      <th scope="row">
                        {formatCalendarDate(e.date)} {e.localTime}
                      </th>
                      <td>{bp.ideas[e.ideaIndex]?.workingTitle ?? "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="small muted">{bp.calendar[0]?.timeZone}</p>
          </Section>

          <div className="row">
            <button type="button" className="btn btn-primary" onClick={() => goTo("review")}>
              {t("common.continue")}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
