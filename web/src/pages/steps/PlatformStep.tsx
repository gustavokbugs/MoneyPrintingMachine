import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Bot, ExternalLink, ShieldCheck, User } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import type { PlatformStep as PlatformSetupStep } from "../../api/types";
import { Badge, Button, ErrorState, Notice, Section, SkeletonCard, TagList, type Tone } from "../../components/ui";
import { formatCalendarDate, t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { YouTubeConnection, parseOutcome } from "./YouTubeConnection";

const automationStyle: Record<PlatformSetupStep["automation"], { tone: Tone; icon: typeof Bot }> = {
  automated: { tone: "info", icon: Bot },
  user_action: { tone: "warning", icon: User },
  verified: { tone: "success", icon: ShieldCheck },
};

const connectionTone: Record<string, Tone> = {
  verified: "success",
  authorized: "warning",
  pending_authorization: "warning",
  revoked: "danger",
  error: "danger",
};

export function PlatformStep({ channel, goTo }: StepProps) {
  const catalog = useQuery({ queryKey: ["platforms"], queryFn: api.platforms, staleTime: Infinity });
  const connections = useQuery({ queryKey: ["connections", channel.id], queryFn: () => api.connections(channel.id) });
  const [params] = useSearchParams();
  const outcome = parseOutcome(params.get("youtube"));

  return (
    <div className="stack">
      <Notice tone="info" title={t("platforms.title")}>
        {t("platforms.intro")}
      </Notice>
      {catalog.isPending && <SkeletonCard lines={5} />}
      {catalog.isError && <ErrorState error={catalog.error} onRetry={() => void catalog.refetch()} />}
      {catalog.data?.map((p) => {
        const conn = connections.data?.find((c) => c.platform === p.platform);
        const status = conn?.status;
        return (
          <Section
            key={p.platform}
            title={p.displayName}
            actions={
              <Badge tone={status ? connectionTone[status] ?? "neutral" : "neutral"} icon={status === "verified" ? ShieldCheck : undefined}>
                {status ? t(`connection.${status}` as MessageKey) : t("platforms.notConnected")}
              </Badge>
            }
          >
            {p.platform === "youtube" ? (
              <YouTubeConnection channel={channel} connection={conn} outcome={outcome} />
            ) : (
              !p.integrationReady && <Notice tone="warning">{t("platforms.notReady", { platform: p.displayName })}</Notice>
            )}
            <h3>{t("platforms.steps")}</h3>
            <ol className="steps-list">
              {p.steps.map((s) => {
                const style = automationStyle[s.automation];
                return (
                  <li key={s.id}>
                    <div className="stack-sm">
                      <div className="row wrap between">
                        <strong>{s.title}</strong>
                        <Badge tone={style.tone} icon={style.icon}>
                          {t(`platforms.auto.${s.automation}` as MessageKey)}
                        </Badge>
                      </div>
                      <p className="small">{s.description}</p>
                      {s.url && (
                        <a href={s.url} target="_blank" rel="noopener noreferrer" className="row small">
                          {t("platforms.open", { platform: p.displayName })}
                          <ExternalLink size={14} aria-hidden="true" />
                          <span className="sr-only">{t("common.newTab")}</span>
                        </a>
                      )}
                    </div>
                  </li>
                );
              })}
            </ol>
            <details>
              <summary>{t("platforms.accountCreation")}</summary>
              <div className="stack-sm">
                <p className="small">{p.accountCreation}</p>
                <h4>{t("platforms.scopes")}</h4>
                <TagList items={p.oauthScopes} />
                <h4>{t("platforms.limits")}</h4>
                <ul className="small">
                  {p.limits.map((l) => (
                    <li key={l}>{l}</li>
                  ))}
                </ul>
                <p className="xs muted">{t("platforms.reviewed", { date: formatCalendarDate(p.lastReviewed) })}</p>
              </div>
            </details>
          </Section>
        );
      })}
      <div className="row">
        <Button variant="primary" icon={ArrowRight} onClick={() => goTo("blueprint")}>
          {t("common.continue")}
        </Button>
      </div>
    </div>
  );
}
