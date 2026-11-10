import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Circle, CircleCheck, Rocket } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { ChannelStatus } from "../../api/types";
import { useConfirm, useToast } from "../../components/feedback";
import { Button, ErrorState, Notice, Section, SkeletonCard, StatusBadge } from "../../components/ui";
import { t, type MessageKey } from "../../i18n";
import type { StepProps } from "../ChannelWizard";
import { useChannelUpdate } from "./shared";

/** Launch states are the primary action; archive is destructive and kept apart. */
const primaryTargets: ChannelStatus[] = ["ready", "active"];

export function ReviewStep({ channel }: StepProps) {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const review = useQuery({
    queryKey: ["launchReview", channel.id, channel.version],
    queryFn: () => api.launchReview(channel.id),
  });
  const [reason, setReason] = useState("");
  const change = useChannelUpdate(channel.id, (to: ChannelStatus) => api.changeStatus(channel.id, to, reason));

  const transition = async (to: ChannelStatus) => {
    if (to === "archived") {
      const ok = await confirm({ title: t("review.archiveTitle"), body: t("review.archiveConfirm"), confirmLabel: t("review.archive"), tone: "danger" });
      if (!ok) return;
    }
    change.mutate(to, {
      onSuccess: () => {
        setReason("");
        toast.success(t("review.moved", { status: t(`status.${to}` as MessageKey) }));
        void qc.invalidateQueries({ queryKey: ["launchReview", channel.id] });
      },
    });
  };

  const targets = channel.allowedTransitions.filter((s) => s !== "archived");
  const canArchive = channel.allowedTransitions.includes("archived");
  const primary = targets.find((s) => primaryTargets.includes(s));

  return (
    <div className="stack">
      {review.isPending && <SkeletonCard lines={6} />}
      {review.isError && <ErrorState error={review.error} onRetry={() => void review.refetch()} />}
      {review.data && (
        <Section title={t("review.title")}>
          <Notice tone={review.data.readyToLaunch ? "success" : "warning"}>{review.data.readyToLaunch ? t("review.ready") : t("review.notReady")}</Notice>
          <div className="review-head">
            {channel.identity && <img src={api.assetUrl(channel.id, "avatar", 0, channel.version)} alt="" width={72} height={72} className="avatar-md" />}
            <div className="stack-sm">
              <h3>{channel.name || t("dashboard.untitled")}</h3>
              {channel.identity?.shortBio && <p className="small">{channel.identity.shortBio}</p>}
              <p className="xs muted">
                {channel.niche} · {channel.language} · {channel.geoTarget}
              </p>
            </div>
          </div>
          <ul className="checklist">
            {review.data.checks.map((c) => {
              const Icon = c.ok ? CircleCheck : Circle;
              return (
                <li key={c.id} className={c.ok ? "ok" : "missing"}>
                  <Icon size={20} aria-hidden="true" />
                  <span>
                    <strong>{c.label}</strong>
                    <span className="sr-only"> ({c.ok ? t("review.complete") : t("review.missing")})</span>
                    {c.detail && <span className="small muted"> — {c.detail}</span>}
                  </span>
                </li>
              );
            })}
          </ul>
        </Section>
      )}

      <Section title={t("review.status")} description={t("review.statusHelp")}>
        <div className="row">
          <span className="small muted">{t("review.current")}</span>
          <StatusBadge status={channel.status} />
        </div>
        <div className="field-group">
          <label htmlFor="status-reason">{t("review.reason")}</label>
          <input id="status-reason" maxLength={500} value={reason} aria-describedby="status-reason-help" onChange={(e) => setReason(e.target.value)} />
          <span id="status-reason-help" className="helper">
            {t("review.reasonHelp")}
          </span>
        </div>
        <div className="row wrap">
          {targets.map((to) => (
            <Button
              key={to}
              variant={to === primary ? "primary" : "secondary"}
              icon={to === primary ? Rocket : undefined}
              loading={change.isPending && change.variables === to}
              disabled={change.isPending}
              onClick={() => void transition(to)}
            >
              {t("review.moveTo", { status: t(`status.${to}` as MessageKey) })}
            </Button>
          ))}
        </div>
        {change.isError && <ErrorState error={change.error} />}
        {canArchive && (
          <div className="row" style={{ borderTop: "1px solid var(--color-border)", paddingTop: "var(--space-4)" }}>
            <Button variant="danger-ghost" icon={Archive} loading={change.isPending && change.variables === "archived"} disabled={change.isPending} onClick={() => void transition("archived")}>
              {t("review.archive")}
            </Button>
          </div>
        )}
      </Section>
    </div>
  );
}
