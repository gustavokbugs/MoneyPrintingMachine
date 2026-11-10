import { ArrowRight, CalendarDays, LayoutList } from "lucide-react";
import { GenerationPanel } from "../../components/GenerationPanel";
import { Button, Section, TagList } from "../../components/ui";
import { useGeneration } from "../../hooks/useGeneration";
import { formatCalendarDate, formatDuration, t } from "../../i18n";
import type { StepProps } from "../ChannelWizard";

export function BlueprintStep({ channel, goTo }: StepProps) {
  const gen = useGeneration(channel.id, "blueprint");
  const bp = channel.blueprint;
  const locked = channel.status === "archived";

  return (
    <div className="stack">
      <GenerationPanel
        title={t("blueprint.title")}
        description={t("blueprint.description")}
        gen={gen}
        provenance={bp?.provenance}
        disabled={gen.running || locked || !channel.strategy}
        emptyIcon={LayoutList}
        emptyText={t("blueprint.empty")}
        hint={t("blueprint.slow")}
      />

      {bp && (
        <>
          <div className="grid-halves">
            <Section title={t("blueprint.priority")}>
              <ol className="small">
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
                    <strong>{f.name}</strong>{" "}
                    <span className="small muted tabular">
                      · {f.aspectRatio} · {formatDuration(f.durationSeconds)}
                    </span>
                    <p className="small muted">{f.description}</p>
                  </li>
                ))}
              </ul>
            </Section>
            <Section title={t("blueprint.hooks")}>
              <TagList items={bp.hooks} />
            </Section>
          </div>

          <Section title={t("blueprint.ideas", { count: bp.ideas.length })} description={t("blueprint.ideasHelp")}>
            <ol className="ideas">
              {bp.ideas.map((idea, i) => (
                <li key={`${i}-${idea.workingTitle}`}>
                  <details>
                    <summary>
                      <span className="muted tabular">{i + 1}.</span>
                      <strong>{idea.workingTitle}</strong>
                      <span className="small muted tabular">
                        {idea.format} · {formatDuration(idea.suggestedDurationSeconds)}
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

          <Section title={t("blueprint.calendar")} description={bp.calendar[0] ? t("blueprint.calendarTz", { tz: bp.calendar[0].timeZone }) : undefined}>
            <div className="table-wrap">
              <table>
                <caption className="sr-only">{t("blueprint.calendar")}</caption>
                <thead>
                  <tr>
                    <th scope="col">
                      <span className="row">
                        <CalendarDays size={14} aria-hidden="true" />
                        {t("blueprint.date")}
                      </span>
                    </th>
                    <th scope="col">{t("blueprint.video")}</th>
                  </tr>
                </thead>
                <tbody>
                  {bp.calendar.map((e) => (
                    <tr key={`${e.date}-${e.ideaIndex}`}>
                      <td className="tabular">
                        {formatCalendarDate(e.date)} {e.localTime}
                      </td>
                      <td>{bp.ideas[e.ideaIndex]?.workingTitle ?? "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Section>

          <div className="row">
            <Button variant="primary" icon={ArrowRight} onClick={() => goTo("review")}>
              {t("common.continue")}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
