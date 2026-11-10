import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { Channel, PlatformConnection } from "../../api/types";
import { ErrorState } from "../../components/ui";
import { t, type MessageKey } from "../../i18n";

export const callbackOutcomes = ["connected", "needs_attention", "denied", "invalid_state", "already_connected", "error"] as const;
export type CallbackOutcome = (typeof callbackOutcomes)[number];

export function parseOutcome(v: string | null): CallbackOutcome | null {
  return v !== null && (callbackOutcomes as readonly string[]).includes(v) ? (v as CallbackOutcome) : null;
}

function Permission({ label, ok }: { label: string; ok: boolean }) {
  return (
    <li>
      <span className={ok ? "chip chip-high" : "chip chip-low"}>
        {label}: {ok ? t("platforms.granted") : t("platforms.missing")}
      </span>
    </li>
  );
}

/** YouTubeConnection connects, verifies, and disconnects a YouTube account. */
export function YouTubeConnection({ channel, connection, outcome }: { channel: Channel; connection?: PlatformConnection; outcome: CallbackOutcome | null }) {
  const qc = useQueryClient();
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
  const verify = useMutation({ mutationFn: () => api.verifyYouTube(channel.id), onSuccess: refresh });
  const disconnect = useMutation({ mutationFn: () => api.disconnectYouTube(channel.id), onSuccess: refresh });

  const configured = integrations.data?.youtubeOAuth.configured === true;
  const locked = channel.status === "archived";
  const busy = start.isPending || verify.isPending || disconnect.isPending || redirecting;

  return (
    <div className="stack-sm">
      {outcome && (
        <div className={outcome === "connected" ? "alert alert-success" : "alert alert-warning"} role="status">
          {t(`platforms.outcome.${outcome}` as MessageKey)}
        </div>
      )}
      {integrations.isSuccess && !configured && <div className="alert alert-warning">{t("platforms.oauthNotConfigured")}</div>}

      {connection?.externalName && (
        <p>
          <strong>{t("platforms.connectedAs", { name: connection.externalName })}</strong>{" "}
          <code className="small">{connection.externalId}</code>
        </p>
      )}
      {connection?.statusReason && <p className="small">{connection.statusReason}</p>}
      {connection && (
        <>
          <h4>{t("platforms.permissions")}</h4>
          <ul className="tags">
            <Permission label={t("platforms.canPublish")} ok={connection.canPublish} />
            <Permission label={t("platforms.canReadAnalytics")} ok={connection.canReadAnalytics} />
          </ul>
        </>
      )}

      <div className="row wrap">
        <button type="button" className="btn btn-primary" disabled={!configured || busy || locked} onClick={() => start.mutate()}>
          {redirecting ? t("platforms.redirecting") : connection ? t("platforms.reconnect") : t("platforms.connect")}
        </button>
        {connection && connection.status !== "pending_authorization" && (
          <button type="button" className="btn btn-secondary" disabled={!configured || busy} onClick={() => verify.mutate()}>
            {t("platforms.verify")}
          </button>
        )}
        {connection && (
          <button
            type="button"
            className="btn btn-ghost"
            disabled={!configured || busy}
            onClick={() => {
              if (window.confirm(t("platforms.disconnectConfirm"))) disconnect.mutate();
            }}
          >
            {t("platforms.disconnect")}
          </button>
        )}
      </div>
      {configured && <p className="small muted">{t("platforms.testingNote")}</p>}
      {start.isError && <ErrorState error={start.error} />}
      {verify.isError && <ErrorState error={verify.error} />}
      {disconnect.isError && <ErrorState error={disconnect.error} />}
    </div>
  );
}
