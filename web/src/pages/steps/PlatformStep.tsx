import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { ErrorState, Loading, Section, TagList } from "../../components/ui";
import { formatCalendarDate, t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { YouTubeConnection, parseOutcome } from "./YouTubeConnection";

export function PlatformStep({ channel, goTo }: StepProps) {
  const catalog = useQuery({ queryKey: ["platforms"], queryFn: api.platforms, staleTime: Infinity });
  const connections = useQuery({ queryKey: ["connections", channel.id], queryFn: () => api.connections(channel.id) });
  const [params] = useSearchParams();
  const outcome = parseOutcome(params.get("youtube"));

  return (
    <div className="stack">
      <Section title={t("platforms.title")}>
        <p>{t("platforms.intro")}</p>
      </Section>
      {catalog.isPending && <Loading />}
      {catalog.isError && <ErrorState error={catalog.error} onRetry={() => void catalog.refetch()} />}
      {catalog.data?.map((p) => {
        const conn = connections.data?.find((c) => c.platform === p.platform);
        return (
          <Section key={p.platform} title={p.displayName}>
            <dl className="fields">
              <div className="field">
                <dt>{t("platforms.connection")}</dt>
                <dd>
                  <span className="chip">{conn ? conn.status.replace(/_/g, " ") : t("platforms.notConnected")}</span>
                </dd>
              </div>
            </dl>
            {p.platform === "youtube" ? (
              <YouTubeConnection channel={channel} connection={conn} outcome={outcome} />
            ) : (
              !p.integrationReady && <div className="alert alert-warning">{t("platforms.notReady", { platform: p.displayName })}</div>
            )}
            <h3>{t("platforms.steps")}</h3>
            <ol className="steps-list">
              {p.steps.map((s) => (
                <li key={s.id}>
                  <div className="row between">
                    <strong>{s.title}</strong>
                    <span className={`chip chip-auto-${s.automation}`}>{t(`platforms.auto.${s.automation}` as MessageKey)}</span>
                  </div>
                  <p className="small">{s.description}</p>
                  {s.url && (
                    <a href={s.url} target="_blank" rel="noopener noreferrer" className="small">
                      {t("platforms.open", { platform: p.displayName })}
                    </a>
                  )}
                </li>
              ))}
            </ol>
            <details>
              <summary>{t("platforms.accountCreation")}</summary>
              <p className="small">{p.accountCreation}</p>
              <h4>{t("platforms.scopes")}</h4>
              <TagList items={p.oauthScopes} />
              <h4>{t("platforms.limits")}</h4>
              <ul className="small">
                {p.limits.map((l) => (
                  <li key={l}>{l}</li>
                ))}
              </ul>
              <p className="small muted">{t("platforms.reviewed", { date: formatCalendarDate(p.lastReviewed) })}</p>
            </details>
          </Section>
        );
      })}
      <div className="row">
        <button type="button" className="btn btn-primary" onClick={() => goTo("blueprint")}>
          {t("common.continue")}
        </button>
      </div>
    </div>
  );
}
