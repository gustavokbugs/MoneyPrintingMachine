import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleCheck, CircleX, LogIn, RefreshCw, Unplug } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { Channel, PlatformConnection } from "../../api/types";
import { useConfirm, useToast } from "../../components/feedback";
import { Button, ErrorState, Notice } from "../../components/ui";
import { t, type MessageKey } from "../../i18n";

export const callbackOutcomes = ["connected", "needs_attention", "denied", "invalid_state", "already_connected", "error"] as const;
export type CallbackOutcome = (typeof callbackOutcomes)[number];

export function parseOutcome(v: string | null): CallbackOutcome | null {
  return v !== null && (callbackOutcomes as readonly string[]).includes(v) ? (v as CallbackOutcome) : null;
}

function Permission({ label, ok }: { label: string; ok: boolean }) {
  const Icon = ok ? CircleCheck : CircleX;
  return (
    <li className="row small">
      <Icon size={16} aria-hidden="true" className={ok ? "tone-text-success" : "tone-text-danger"} />
      {label}: {ok ? t("platforms.granted") : t("platforms.missing")}
    </li>
  );
}

/** YouTubeConnection connects, verifies, and disconnects a YouTube account. */
export function YouTubeConnection({ channel, connection, outcome }: { channel: Channel; connection?: PlatformConnection; outcome: CallbackOutcome | null }) {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const integrations = useQuery({ queryKey: ["integrations"], queryFn: api.integrations });
  const [redirecting, setRedirecting] = useState(false);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["connections", channel.id] });
    void qc.invalidateQueries({ queryKey: ["channels"] });
    void qc.invalidateQueries({ queryKey: ["launchReview", channel.id] });
  };

  const start = useMutation({
    mutationFn: () => api.startYouTube(channel.id),
    onSuccess: (url) => {
      setRedirecting(true);
      window.location.assign(url);
    },
  });
  const verify = useMutation({
    mutationFn: () => api.verifyYouTube(channel.id),
    onSuccess: (c) => {
      refresh();
      if (c.status === "verified") toast.success(t("platforms.verifiedToast"));
    },
  });
  const disconnect = useMutation({
    mutationFn: () => api.disconnectYouTube(channel.id),
    onSuccess: () => {
      refresh();
      toast.success(t("platforms.disconnectedToast"));
    },
  });

  const configured = integrations.data?.youtubeOAuth.configured === true;
  const locked = channel.status === "archived";
  const busy = start.isPending || verify.isPending || disconnect.isPending || redirecting;

  const onDisconnect = async () => {
    const ok = await confirm({
      title: t("platforms.disconnectTitle"),
      body: t("platforms.disconnectConfirm"),
      confirmLabel: t("platforms.disconnect"),
      tone: "danger",
    });
    if (ok) disconnect.mutate();
  };

  return (
    <div className="stack-sm">
      {outcome && (
        <Notice tone={outcome === "connected" ? "success" : "warning"} live>
          {t(`platforms.outcome.${outcome}` as MessageKey)}
        </Notice>
      )}
      {integrations.isSuccess && !configured && <Notice tone="warning">{t("platforms.oauthNotConfigured")}</Notice>}

      {connection?.externalName && (
        <p>
          <strong>{t("platforms.connectedAs", { name: connection.externalName })}</strong> <code className="xs muted">{connection.externalId}</code>
        </p>
      )}
      {connection?.statusReason && <p className="small">{connection.statusReason}</p>}
      {connection && (
        <div className="stack-sm">
          <h4>{t("platforms.permissions")}</h4>
          <ul className="plain-list">
            <Permission label={t("platforms.canPublish")} ok={connection.canPublish} />
            <Permission label={t("platforms.canReadAnalytics")} ok={connection.canReadAnalytics} />
          </ul>
        </div>
      )}

      <div className="row wrap">
        <Button variant="primary" icon={LogIn} loading={start.isPending || redirecting} disabled={!configured || busy || locked} onClick={() => start.mutate()}>
          {redirecting ? t("platforms.redirecting") : connection ? t("platforms.reconnect") : t("platforms.connect")}
        </Button>
        {connection && connection.status !== "pending_authorization" && (
          <Button icon={RefreshCw} loading={verify.isPending} disabled={!configured || busy} onClick={() => verify.mutate()}>
            {t("platforms.verify")}
          </Button>
        )}
        {connection && (
          <Button variant="danger-ghost" icon={Unplug} loading={disconnect.isPending} disabled={!configured || busy} onClick={() => void onDisconnect()}>
            {t("platforms.disconnect")}
          </Button>
        )}
      </div>
      {configured && <p className="xs muted">{t("platforms.testingNote")}</p>}
      {start.isError && <ErrorState error={start.error} />}
      {verify.isError && <ErrorState error={verify.error} onRetry={() => verify.mutate()} />}
      {disconnect.isError && <ErrorState error={disconnect.error} />}
    </div>
  );
}
