import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { ChannelStatus } from "../../api/types";
import { ErrorState, Loading, Section, StatusBadge } from "../../components/ui";
import { t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

export function ReviewStep({ channel }: StepProps) {
  const qc = useQueryClient();
  const review = useQuery({
    queryKey: ["launchReview", channel.id, channel.version],
    queryFn: () => api.launchReview(channel.id),
  });
  const [reason, setReason] = useState("");
  const change = useChannelUpdate(channel.id, (to: ChannelStatus) => api.changeStatus(channel.id, to, reason));

  const transition = (to: ChannelStatus) => {
    if (to === "archived" && !window.confirm(t("review.archiveConfirm"))) return;
    change.mutate(to, {
      onSuccess: () => {
        setReason("");
        void qc.invalidateQueries({ queryKey: ["launchReview", channel.id] });
      },
    });
  };

  return (
    <div className="stack">
      <Section title={t("review.title")}>
        {review.isPending && <Loading />}
        {review.isError && <ErrorState error={review.error} onRetry={() => void review.refetch()} />}
        {review.data && (
          <>
            <div className={review.data.readyToLaunch ? "alert alert-success" : "alert alert-warning"}>
              {review.data.readyToLaunch ? t("review.ready") : t("review.notReady")}
            </div>
            <div className="review-head">
              {channel.identity && (
                <img src={api.assetUrl(channel.id, "avatar", 0, channel.version)} alt="" className="avatar-md" />
              )}
              <div>
                <h3>{channel.name || t("dashboard.untitled")}</h3>
                <p>{channel.identity?.shortBio}</p>
                <p className="small muted">
                  {channel.niche} · {channel.language} · {channel.geoTarget}
                </p>
              </div>
            </div>
            <ul className="checklist">
              {review.data.checks.map((c) => (
                <li key={c.id} className={c.ok ? "ok" : "missing"}>
                  <span aria-hidden="true">{c.ok ? "✓" : "○"}</span>
                  <span>
                    <strong>{c.label}</strong>
                    {c.detail && <span className="small muted"> — {c.detail}</span>}
                  </span>
                  <span className="sr-only">{c.ok ? t("review.complete") : t("review.missing")}</span>
                </li>
              ))}
            </ul>
          </>
        )}
      </Section>

      <Section title={t("review.status")}>
        <p>
          <StatusBadge status={channel.status} />
        </p>
        <label htmlFor="status-reason">{t("review.reason")}</label>
        <input id="status-reason" maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        <div className="row wrap">
          {channel.allowedTransitions.map((to) => (
            <button
              key={to}
              type="button"
              className={to === "ready" || to === "active" ? "btn btn-primary" : "btn btn-secondary"}
              disabled={change.isPending}
              onClick={() => transition(to)}
            >
              {t(`status.${to}` as MessageKey)}
            </button>
          ))}
        </div>
        {change.isError && <ErrorState error={change.error} />}
      </Section>
    </div>
  );
}
