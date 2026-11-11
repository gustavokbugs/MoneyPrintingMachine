import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Upload } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { api, newIdempotencyKey } from "../api/client";
import type { Privacy, Publication, Video } from "../api/types";
import { formatDateTime, t } from "../i18n";
import { useJob } from "../hooks/useJob";
import { useConfirm, useToast } from "./feedback";
import { Badge, Button, JobProgress, Notice, Section, errorMessage } from "./ui";

const privacies: readonly Privacy[] = ["private", "unlisted", "public"];

function PublicationRow({ p }: { p: Publication }) {
  const tone = p.status === "published" ? "success" : p.status === "failed" ? "danger" : "warning";
  return (
    <li className="subtle-panel">
      <div className="row between wrap">
        <span className="row">
          <Badge tone={tone}>{t(`publish.status.${p.status}`)}</Badge>
          <span className="small">
            YouTube · {t(`publish.privacy.${p.privacy}`)} · {formatDateTime(p.updatedAt)}
          </span>
        </span>
        {p.url && (
          <a className="btn btn-ghost btn-sm" href={p.url} target="_blank" rel="noreferrer noopener">
            <ExternalLink size={16} aria-hidden="true" />
            {t("publish.open")}
          </a>
        )}
      </div>
      {p.error && <p className="xs muted">{p.error}</p>}
    </li>
  );
}

/** PublishPanel uploads a produced video to the channel's connected YouTube account. */
export function PublishPanel({ video }: { video: Video }) {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [privacy, setPrivacy] = useState<Privacy>("private");
  const [jobId, setJobId] = useState<string | null>(null);
  const conns = useQuery({ queryKey: ["connections", video.channelId], queryFn: () => api.connections(video.channelId) });
  const pubs = useQuery({ queryKey: ["publications", video.id], queryFn: () => api.publications(video.id) });
  const job = useJob(jobId, () => {
    void qc.invalidateQueries({ queryKey: ["publications", video.id] });
    toast.success(t("publish.done"));
  });
  const start = useMutation({
    mutationFn: () => api.publishVideo(video.id, privacy, newIdempotencyKey()),
    onSuccess: (j) => setJobId(j.id),
    onError: (err) => toast.error(errorMessage(err)),
  });

  const yt = conns.data?.find((c) => c.platform === "youtube");
  const connected = yt?.status === "verified" && yt.canPublish;
  const items = pubs.data ?? [];
  const published = items.some((p) => p.status === "published");
  const uploading = items.some((p) => p.status === "uploading") || start.isPending || job.data?.status === "queued" || job.data?.status === "running";

  const onPublish = async () => {
    const ok = await confirm({
      title: t("publish.confirmTitle"),
      body: t("publish.confirmBody", { title: video.title, privacy: t(`publish.privacy.${privacy}`), account: yt?.externalName ?? "YouTube" }),
      confirmLabel: t("publish.upload"),
      tone: "primary",
    });
    if (ok) start.mutate();
  };

  return (
    <Section title={t("publish.title")} description={t("publish.help")}>
      {conns.isSuccess && !connected && (
        <Notice tone="warning">
          {t("publish.notConnected")} <Link to={`/channels/${video.channelId}`}>{t("publish.connect")}</Link>
        </Notice>
      )}
      {video.status !== "produced" && <p className="small muted">{t("publish.needsRender")}</p>}
      <div className="row wrap">
        <label htmlFor="privacy" className="label">
          {t("publish.privacyLabel")}
        </label>
        <select
          id="privacy"
          value={privacy}
          disabled={uploading}
          onChange={(e) => {
            const next = privacies.find((p) => p === e.target.value);
            if (next) setPrivacy(next);
          }}
        >
          {privacies.map((p) => (
            <option key={p} value={p}>
              {t(`publish.privacy.${p}`)}
            </option>
          ))}
        </select>
        <Button variant="primary" icon={Upload} loading={uploading} disabled={!connected || published || video.status !== "produced"} onClick={() => void onPublish()}>
          {t("publish.upload")}
        </Button>
      </div>
      {jobId && <JobProgress job={job.data} error={job.error} />}
      {items.length > 0 && (
        <ol className="plain-list">
          {items.map((p) => (
            <PublicationRow key={p.id} p={p} />
          ))}
        </ol>
      )}
      <p className="xs muted">{t("publish.policy")}</p>
    </Section>
  );
}
