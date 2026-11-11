import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCheck, ChevronRight, CircleCheck, Download, FileText, Film, RotateCcw, ShieldCheck, Sparkles, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, newIdempotencyKey } from "../api/client";
import type { ClaimStatus, Video, VideoScript, VisualStyle } from "../api/types";
import { EditableText } from "../components/EditableText";
import { PublishPanel } from "../components/PublishPanel";
import { useToast } from "../components/feedback";
import { Badge, Button, Empty, ErrorState, JobProgress, Notice, ProvenanceNote, Section, SkeletonCard, errorMessage } from "../components/ui";
import { useJob } from "../hooks/useJob";
import { formatDateTime, formatDuration, t } from "../i18n";
import { VideoStatusBadge } from "./ChannelVideos";
import { DialogueControls, speakerRole } from "../components/DialogueControls";

function useVideoJob(video: Video, start: (key: string) => ReturnType<typeof api.generateVideoScript>, doneMessage: string) {
  const qc = useQueryClient();
  const toast = useToast();
  const [jobId, setJobId] = useState<string | null>(null);
  const job = useJob(jobId, () => {
    void qc.invalidateQueries({ queryKey: ["video", video.id] });
    void qc.invalidateQueries({ queryKey: ["videos", video.channelId] });
    toast.success(doneMessage);
  });
  const m = useMutation({ mutationFn: () => start(newIdempotencyKey()), onSuccess: (j) => setJobId(j.id), onError: (err) => toast.error(errorMessage(err)) });
  const running = m.isPending || job.data?.status === "queued" || job.data?.status === "running";
  return { run: () => m.mutate(), running, job, jobId };
}

function ClaimsReview({ video, onChange }: { video: Video; onChange: (v: Video) => void }) {
  const toast = useToast();
  const review = useMutation({
    mutationFn: ({ i, status }: { i: number; status: ClaimStatus }) => api.reviewClaim(video.id, i, status),
    onSuccess: onChange,
    onError: (err) => toast.error(errorMessage(err)),
  });
  const claims = video.script?.claims ?? [];
  if (claims.length === 0) return <p className="small muted">{t("video.noClaims")}</p>;
  return (
    <ol className="plain-list">
      {claims.map((c, i) => (
        <li key={`${i}-${c.text}`} className="subtle-panel">
          <div className="row between wrap">
            <span className={c.status === "removed" ? "muted" : undefined} style={c.status === "removed" ? { textDecoration: "line-through" } : undefined}>
              {c.text}
            </span>
            <Badge tone={c.status === "verified" ? "success" : c.status === "removed" ? "neutral" : "warning"}>{t(`video.claim.${c.status}`)}</Badge>
          </div>
          {c.sourceHint && <p className="xs muted">{t("video.checkSource", { source: c.sourceHint })}</p>}
          <div className="row wrap">
            <Button size="sm" icon={CircleCheck} disabled={c.status === "verified" || review.isPending} onClick={() => review.mutate({ i, status: "verified" })}>
              {t("video.markVerified")}
            </Button>
            <Button size="sm" variant="ghost" icon={Trash2} disabled={c.status === "removed" || review.isPending} onClick={() => review.mutate({ i, status: "removed" })}>
              {t("video.markRemoved")}
            </Button>
            {c.status !== "unverified" && (
              <Button size="sm" variant="ghost" icon={RotateCcw} disabled={review.isPending} onClick={() => review.mutate({ i, status: "unverified" })}>
                {t("video.reopen")}
              </Button>
            )}
          </div>
        </li>
      ))}
    </ol>
  );
}

export function VideoDetail() {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const toast = useToast();
  const video = useQuery({
    queryKey: ["video", id],
    queryFn: () => api.getVideo(id),
    refetchInterval: (q) => (q.state.data?.status === "scripting" || q.state.data?.status === "producing" ? 3000 : false),
  });
  const status = useQuery({ queryKey: ["productionStatus"], queryFn: api.productionStatus });
  const setVideo = (v: Video) => qc.setQueryData(["video", id], v);
  const save = useMutation({ mutationFn: (s: VideoScript) => api.putVideoScript(id, s), onSuccess: setVideo });
  const approve = useMutation({
    mutationFn: () => api.approveVideo(id),
    onSuccess: (v) => {
      setVideo(v);
      toast.success(t("video.approved"));
    },
    onError: (err) => toast.error(errorMessage(err)),
  });
  const voice = useMutation({ mutationFn: (vid: number) => api.setVoice(id, vid), onSuccess: setVideo, onError: (err) => toast.error(errorMessage(err)) });
  const style = useMutation({ mutationFn: (st: VisualStyle) => api.setVisualStyle(id, st), onSuccess: setVideo, onError: (err) => toast.error(errorMessage(err)) });

  if (video.isPending) return <SkeletonCard lines={6} />;
  if (video.isError) return <ErrorState error={video.error} onRetry={() => void video.refetch()} />;
  return <VideoWorkspace video={video.data} tools={status.data} save={save} approve={approve} voice={voice} style={style} setVideo={setVideo} />;
}

interface WorkspaceProps {
  video: Video;
  tools: Awaited<ReturnType<typeof api.productionStatus>> | undefined;
  save: { mutateAsync: (s: VideoScript) => Promise<Video> };
  approve: { mutate: () => void; isPending: boolean };
  voice: { mutate: (id: number) => void; isPending: boolean };
  style: { mutate: (s: VisualStyle) => void; isPending: boolean };
  setVideo: (v: Video) => void;
}

function VideoWorkspace({ video: v, tools, save, approve, voice, style, setVideo }: WorkspaceProps) {
  const scriptJob = useVideoJob(v, (k) => api.generateVideoScript(v.id, k), t("video.scriptDone"));
  const produceJob = useVideoJob(v, (k) => api.produceVideo(v.id, k), t("video.produced"));
  const sc = v.script;
  const unresolved = sc ? sc.claims.filter((c) => c.status === "unverified").length : 0;
  const busy = v.status === "scripting" || v.status === "producing";
  const editSection = (i: number, narration: string) => (sc ? save.mutateAsync({ ...sc, sections: sc.sections.map((s, j) => (j === i ? { ...s, narration } : s)) }) : Promise.resolve());
  const canProduce = (v.status === "approved" || v.status === "produced" || (v.status === "failed" && sc !== null && unresolved === 0)) && tools?.ready === true;

  return (
    <>
      <nav aria-label={t("wizard.breadcrumb")} className="breadcrumb">
        <ol>
          <li>
            <Link to={`/channels/${v.channelId}/videos`}>{t("videos.title")}</Link>
            <ChevronRight size={14} aria-hidden="true" />
          </li>
          <li aria-current="page">{v.title}</li>
        </ol>
      </nav>
      <div className="page-header">
        <div>
          <h1>{v.title}</h1>
          <p className="small muted tabular">
            {v.format} · {v.aspectRatio} · {t("video.target", { duration: formatDuration(v.targetSeconds) })}
          </p>
        </div>
        <VideoStatusBadge status={v.status} />
      </div>
      {v.status === "failed" && v.statusReason && <ErrorState error={new Error(v.statusReason)} />}

      <Section
        title={t("video.scriptTitle")}
        description={t("video.scriptHelp")}
        actions={
          <Button variant={sc ? "secondary" : "primary"} icon={Sparkles} loading={scriptJob.running || v.status === "scripting"} disabled={busy} onClick={scriptJob.run}>
            {sc ? t("common.regenerate") : t("video.generateScript")}
          </Button>
        }
      >
        <DialogueControls video={v} tools={tools} disabled={busy} onSaved={setVideo} />
        {scriptJob.jobId && <JobProgress job={scriptJob.job.data} error={scriptJob.job.error} />}
        {!sc ? (
          <Empty icon={FileText}>
            <p>{t("video.noScript")}</p>
          </Empty>
        ) : (
          <>
            <ProvenanceNote p={sc.provenance} />
            <p className="small muted tabular">{t("video.estimate", { duration: formatDuration(sc.estimatedSeconds || 0) })}</p>
            <dl className="fields">
              <div className="field">
                <dt>{t("video.hook")}</dt>
                <dd>{sc.hook}</dd>
              </div>
              {sc.sections.map((s, i) => (
                <EditableText key={`${i}-${s.heading}`} label={`${i + 1}. ${speakerRole(tools?.dialogueModes ?? [], v, s.speaker) ? `[${speakerRole(tools?.dialogueModes ?? [], v, s.speaker)}] ` : ""}${s.heading || s.onScreenText}`} value={s.narration} multiline maxLength={4000} onSave={(val) => editSection(i, val)} />
              ))}
              <div className="field">
                <dt>{t("video.outro")}</dt>
                <dd>{sc.outro}</dd>
              </div>
            </dl>
          </>
        )}
      </Section>

      {sc && (
        <Section title={t("video.claimsTitle")} description={t("video.claimsHelp")}>
          <ClaimsReview video={v} onChange={setVideo} />
          <div className="row wrap">
            <Button variant="primary" icon={ShieldCheck} loading={approve.isPending} disabled={v.status !== "script_ready" || unresolved > 0} onClick={() => approve.mutate()}>
              {t("video.approve")}
            </Button>
            {unresolved > 0 && <span className="small muted">{t("video.unresolved", { count: unresolved })}</span>}
            {(v.status === "approved" || v.status === "produced") && (
              <span className="row small">
                <CheckCheck size={16} aria-hidden="true" className="tone-text-success" />
                {t("video.isApproved")}
              </span>
            )}
          </div>
        </Section>
      )}

      {sc && (
        <Section title={t("video.produceTitle")} description={t("video.produceHelp")}>
          {tools && !tools.ready && <Notice tone="warning">{t("video.toolsMissing", { missing: tools.missing.join(", ") })}</Notice>}
          {tools?.ready && !tools.animatedScenes && <Notice tone="info">{t("video.noAnimation")}</Notice>}
          <div className="row wrap">
            <label htmlFor="voice" className="label">
              {t("video.voice")}
            </label>
            <select id="voice" value={v.voiceId} disabled={busy || voice.isPending} onChange={(e) => voice.mutate(Number(e.target.value))}>
              {(tools?.voices ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
            <label htmlFor="visual-style" className="label">
              {t("video.style")}
            </label>
            <select id="visual-style" value={v.visualStyle} disabled={busy || style.isPending} onChange={(e) => {
                const next = (tools?.styles ?? []).find((o) => o === e.target.value);
                if (next) style.mutate(next);
              }}>
              {(tools?.styles ?? ["auto"]).map((o) => (
                <option key={o} value={o}>
                  {t(`video.style.${o}`)}
                </option>
              ))}
            </select>
            <Button variant="primary" icon={Film} loading={produceJob.running || v.status === "producing"} disabled={!canProduce || busy} onClick={produceJob.run}>
              {v.output ? t("video.reproduce") : t("video.produce")}
            </Button>
          </div>
          <p className="xs muted">{t(`video.styleHelp.${v.visualStyle}`)}</p>
          {v.visualStyle === "footage" && tools && !tools.stockFootage && <Notice tone="info">{t("video.footageNoKey")}</Notice>}
          {produceJob.jobId && <JobProgress job={produceJob.job.data} error={produceJob.job.error} />}
          {v.output && (
            <div className="stack-sm">
              <video
                key={v.version}
                controls
                preload="metadata"
                src={api.videoMediaUrl(v.id, "video.mp4", v.version)}
                style={{ width: "100%", maxWidth: v.aspectRatio === "9:16" ? 360 : 960, aspectRatio: v.aspectRatio === "9:16" ? "9 / 16" : "16 / 9", background: "#000", borderRadius: 8 }}
              >
                <track kind="captions" srcLang="en" label="English" />
              </video>
              <p className="xs muted tabular">
                {t("video.rendered", { duration: formatDuration(Math.round(v.output.durationSeconds)), size: `${v.output.width}×${v.output.height}`, visuals: visualsLabel(v.output.visuals), date: formatDateTime(v.output.renderedAt) })}
              </p>
              <div className="row wrap">
                <a className="btn btn-secondary btn-sm" href={api.videoMediaUrl(v.id, "video.mp4", v.version)} download={`${v.title}.mp4`}>
                  <Download size={16} aria-hidden="true" />
                  {t("video.downloadVideo")}
                </a>
                <a className="btn btn-ghost btn-sm" href={api.videoMediaUrl(v.id, "captions.srt", v.version)} download="captions.srt">
                  <Download size={16} aria-hidden="true" />
                  {t("video.downloadCaptions")}
                </a>
              </div>
              {v.output.music && (
                <p className="xs muted">
                  {t("video.soundtrack", { music: v.output.music })} {v.output.soundEffects ? t("video.soundEffects", { count: v.output.soundEffects }) : null}
                </p>
              )}
              {v.output.footage && v.output.footage.length > 0 && (
                <details className="xs muted">
                  <summary>{t("video.footageCredits", { count: v.output.footage.length })}</summary>
                  <ul>
                    {v.output.footage.map((c) => (
                      <li key={c.id}>
                        <a href={safeHttps(c.pageUrl)} target="_blank" rel="noreferrer">
                          {c.title ? `${c.title} · ` : ""}
                          {c.author || t("video.footageUnknownAuthor")}
                        </a>
                        {c.license ? ` (${c.license})` : null}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {v.output.visualsNote && <Notice tone="warning">{v.output.visualsNote}</Notice>}
              {v.output.syntheticVoice && <Notice tone="info">{t("video.disclosure")}</Notice>}
            </div>
          )}
        </Section>
      )}

      {v.output && <PublishPanel video={v} />}
    </>
  );
}

function visualsLabel(visuals: string | undefined): string {
  if (visuals?.startsWith("animated:")) {
    const st = visuals.slice("animated:".length);
    const known: readonly VisualStyle[] = ["minimal", "documentary", "bold", "tech", "gameplay"];
    const match = known.find((k) => k === st);
    return t("video.visuals.animated", { style: match ? t(`video.style.${match}`) : st });
  }
  if (visuals === "footage:pexels") return t("video.visuals.footage");
  if (visuals === "footage:commons") return t("video.visuals.footageCommons");
  if (visuals === "footage:mixed") return t("video.visuals.footageMixed");
  return t("video.visuals.solid");
}

/** safeHttps keeps only https links from untrusted provider data. */
function safeHttps(url: string): string | undefined {
  try {
    return new URL(url).protocol === "https:" ? url : undefined;
  } catch {
    return undefined;
  }
}
