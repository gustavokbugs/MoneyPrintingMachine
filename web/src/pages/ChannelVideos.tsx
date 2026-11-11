import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Clapperboard, Plus } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Video, VideoStatus } from "../api/types";
import { useToast } from "../components/feedback";
import { Badge, Button, Empty, ErrorState, SkeletonCard, errorMessage, type Tone } from "../components/ui";
import { formatDuration, t, type MessageKey } from "../i18n";

export const videoTone: Record<VideoStatus, Tone> = {
  planned: "neutral",
  scripting: "info",
  script_ready: "warning",
  approved: "info",
  producing: "info",
  produced: "success",
  failed: "danger",
};

export function VideoStatusBadge({ status }: { status: VideoStatus }) {
  return <Badge tone={videoTone[status]}>{t(`video.status.${status}` as MessageKey)}</Badge>;
}

export function ChannelVideos() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const channel = useQuery({ queryKey: ["channel", id], queryFn: () => api.getChannel(id) });
  const videos = useQuery({ queryKey: ["videos", id], queryFn: () => api.listVideos(id) });
  const create = useMutation({
    mutationFn: (idx: number) => api.createVideo(id, idx),
    onSuccess: (v) => {
      void qc.invalidateQueries({ queryKey: ["videos", id] });
      toast.success(t("videos.created"));
      navigate(`/videos/${v.id}`);
    },
    onError: (err) => toast.error(errorMessage(err)),
  });

  if (channel.isPending || videos.isPending) return <SkeletonCard lines={5} />;
  if (channel.isError) return <ErrorState error={channel.error} onRetry={() => void channel.refetch()} />;
  if (videos.isError) return <ErrorState error={videos.error} onRetry={() => void videos.refetch()} />;
  const c = channel.data;
  const byIdea = new Map<number, Video>(videos.data.map((v) => [v.ideaIndex, v]));
  const ideas = c.blueprint?.ideas ?? [];

  return (
    <>
      <nav aria-label={t("wizard.breadcrumb")} className="breadcrumb">
        <ol>
          <li>
            <Link to={`/channels/${c.id}`}>{c.name || c.niche}</Link>
            <ChevronRight size={14} aria-hidden="true" />
          </li>
          <li aria-current="page">{t("videos.title")}</li>
        </ol>
      </nav>
      <div className="page-header">
        <div>
          <h1>{t("videos.title")}</h1>
          <p className="muted">{t("videos.subtitle")}</p>
        </div>
      </div>
      {ideas.length === 0 ? (
        <Empty
          icon={Clapperboard}
          action={
            <Link className="btn btn-primary" to={`/channels/${c.id}?step=blueprint`}>
              {t("videos.toBlueprint")}
            </Link>
          }
        >
          <p>{t("videos.noBlueprint")}</p>
        </Empty>
      ) : (
        <section className="card" aria-label={t("videos.title")}>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">#</th>
                  <th scope="col">{t("videos.col.idea")}</th>
                  <th scope="col">{t("videos.col.format")}</th>
                  <th scope="col">{t("videos.col.status")}</th>
                  <th scope="col">
                    <span className="sr-only">{t("trends.col.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {ideas.map((idea, i) => {
                  const v = byIdea.get(i);
                  return (
                    <tr key={`${i}-${idea.workingTitle}`}>
                      <td className="tabular muted">{i + 1}</td>
                      <th scope="row">{idea.workingTitle}</th>
                      <td className="small tabular">
                        {idea.format} · {formatDuration(idea.suggestedDurationSeconds)}
                      </td>
                      <td>{v ? <VideoStatusBadge status={v.status} /> : <span className="muted small">{t("videos.notStarted")}</span>}</td>
                      <td>
                        {v ? (
                          <Link className="btn btn-ghost btn-sm" to={`/videos/${v.id}`}>
                            {t("videos.open")}
                          </Link>
                        ) : (
                          <Button size="sm" icon={Plus} loading={create.isPending && create.variables === i} disabled={create.isPending} onClick={() => create.mutate(i)}>
                            {t("videos.create")}
                          </Button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
