import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChartNoAxesColumn, ChevronDown, ChevronUp, Eye, Newspaper, Plus, RefreshCw, Rss, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { api, newIdempotencyKey } from "../api/client";
import type { Feed, FeedStatus, RefreshResult, Watch } from "../api/types";
import { useConfirm, useToast } from "../components/feedback";
import { Badge, Button, Empty, ErrorState, Notice, Section, Skeleton, errorMessage, type Tone } from "../components/ui";
import { useJob } from "../hooks/useJob";
import { formatDateTime, t, type MessageKey } from "../i18n";

const num = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });

const feedTone: Record<FeedStatus, Tone> = { pending: "neutral", ok: "success", not_modified: "success", error: "danger" };

function fmt(v: number | null, suffix = ""): string {
  return v === null ? "—" : `${num.format(v)}${suffix}`;
}

function Trajectory({ watch }: { watch: Watch }) {
  const q = useQuery({ queryKey: ["trajectory", watch.id, watch.lastMeasuredAt], queryFn: () => api.trajectory(watch) });
  if (q.isPending) return <Skeleton height={80} />;
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (q.data.length === 0) return <p className="small muted">{t("trends.noMeasurements")}</p>;
  return (
    <div className="table-wrap">
      <table>
        <caption className="sr-only">{t("trends.trajectoryCaption", { query: watch.query })}</caption>
        <thead>
          <tr>
            <th scope="col">{t("trends.col.measured")}</th>
            <th scope="col">{t("launchpad.medianViewsPerDay")}</th>
            <th scope="col">{t("launchpad.demand")}</th>
            <th scope="col">{t("launchpad.competition")}</th>
            <th scope="col">{t("launchpad.momentum")}</th>
          </tr>
        </thead>
        <tbody>
          {[...q.data].reverse().map((p) => (
            <tr key={p.collectedAt}>
              <td className="tabular">{formatDateTime(p.collectedAt)}</td>
              <td className="tabular">{compact.format(p.medianViewsPerDay)}</td>
              <td className="tabular">{fmt(p.scores.demand)}</td>
              <td className="tabular">{fmt(p.scores.competition)}</td>
              <td className="tabular">{fmt(p.scores.momentumPercent, "%")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function WatchRow({ watch, onRemove, removing }: { watch: Watch; onRemove: () => void; removing: boolean }) {
  const [open, setOpen] = useState(false);
  const panelId = `trajectory-${watch.id}`;
  return (
    <>
      <tr>
        <th scope="row">
          <span className="stack-sm">
            <span>{watch.query}</span>
            {watch.lastError && <span className="xs error-text">{watch.lastError}</span>}
          </span>
        </th>
        <td>
          <Badge tone={watch.source === "channel" ? "info" : "neutral"}>{t(`trends.source.${watch.source}` as MessageKey)}</Badge>
          {!watch.active && (
            <>
              {" "}
              <Badge tone="neutral">{t("trends.paused")}</Badge>
            </>
          )}
        </td>
        <td className="tabular">{watch.region}</td>
        <td className="tabular small">{watch.lastMeasuredAt ? formatDateTime(watch.lastMeasuredAt) : <span className="muted">{t("trends.never")}</span>}</td>
        <td className="tabular small">{watch.active ? formatDateTime(watch.nextDueAt) : "—"}</td>
        <td>
          <span className="row">
            <Button size="sm" variant="ghost" icon={open ? ChevronUp : ChevronDown} aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((v) => !v)}>
              {t("trends.history")}
            </Button>
            {watch.source === "manual" && (
              <button type="button" className="btn btn-danger-ghost btn-sm btn-icon" aria-label={t("trends.removeWatch", { query: watch.query })} disabled={removing} onClick={onRemove}>
                <Trash2 size={16} aria-hidden="true" />
              </button>
            )}
          </span>
        </td>
      </tr>
      {open && (
        <tr id={panelId}>
          <td colSpan={6}>
            <Trajectory watch={watch} />
          </td>
        </tr>
      )}
    </>
  );
}

function FeedItem({ feed, onDelete }: { feed: Feed; onDelete: () => void }) {
  return (
    <li className="row between wrap">
      <span className="stack-sm" style={{ minWidth: 0 }}>
        <strong className="small">{feed.title || feed.url}</strong>
        <code className="xs muted">{feed.url}</code>
        {feed.lastStatus === "error" && <span className="xs error-text">{feed.lastError}</span>}
      </span>
      <span className="row wrap">
        <Badge tone={feedTone[feed.lastStatus]}>{t(`trends.feedStatus.${feed.lastStatus}` as MessageKey)}</Badge>
        <span className="xs muted tabular">{t("trends.itemCount", { count: feed.itemCount })}</span>
        <button type="button" className="btn btn-danger-ghost btn-sm btn-icon" aria-label={t("trends.removeFeed", { name: feed.title || feed.url })} onClick={onDelete}>
          <Trash2 size={16} aria-hidden="true" />
        </button>
      </span>
    </li>
  );
}

export function Trends() {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [query, setQuery] = useState("");
  const [region, setRegion] = useState("US");
  const [feedUrl, setFeedUrl] = useState("");
  const [refreshJob, setRefreshJob] = useState<string | null>(null);

  const integrations = useQuery({ queryKey: ["integrations"], queryFn: api.integrations });
  const watchlist = useQuery({ queryKey: ["watchlist"], queryFn: api.watchlist });
  const feeds = useQuery({ queryKey: ["feeds"], queryFn: api.feeds, refetchInterval: 60_000 });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["watchlist"] });
    void qc.invalidateQueries({ queryKey: ["feeds"] });
    void qc.invalidateQueries({ queryKey: ["integrations"] });
  };

  const job = useJob(refreshJob, (j) => {
    invalidate();
    const r = j.result as RefreshResult | undefined;
    toast.success(r ? t("trends.refreshDone", { measured: r.measured, feeds: r.feedsFetched }) : t("trends.refreshDoneShort"));
    if (r?.stopped) toast.error(r.stopped);
  });
  const refresh = useMutation({
    mutationFn: () => api.refreshTrends(newIdempotencyKey()),
    onSuccess: (j) => setRefreshJob(j.id),
    onError: (err) => toast.error(errorMessage(err)),
  });
  const refreshing = refresh.isPending || job.data?.status === "queued" || job.data?.status === "running";

  const addWatch = useMutation({
    mutationFn: () => api.addWatch(query.trim(), region.trim()),
    onSuccess: (w) => {
      setQuery("");
      invalidate();
      toast.success(t("trends.watchAdded", { query: w.query }));
    },
  });
  const removeWatch = useMutation({
    mutationFn: (id: number) => api.deleteWatch(id),
    onSuccess: invalidate,
    onError: (err) => toast.error(errorMessage(err)),
  });
  const addFeed = useMutation({
    mutationFn: () => api.addFeed(feedUrl.trim()),
    onSuccess: () => {
      setFeedUrl("");
      invalidate();
      toast.success(t("trends.feedAdded"));
    },
  });
  const removeFeed = useMutation({
    mutationFn: (id: number) => api.deleteFeed(id),
    onSuccess: invalidate,
    onError: (err) => toast.error(errorMessage(err)),
  });

  const onAddWatch = (e: FormEvent) => {
    e.preventDefault();
    if (query.trim()) addWatch.mutate();
  };
  const onAddFeed = (e: FormEvent) => {
    e.preventDefault();
    if (feedUrl.trim()) addFeed.mutate();
  };
  const confirmRemoveWatch = async (w: Watch) => {
    if (await confirm({ title: t("trends.removeWatchTitle"), body: t("trends.removeWatchBody", { query: w.query }), confirmLabel: t("common.delete"), tone: "danger" })) {
      removeWatch.mutate(w.id);
    }
  };
  const confirmRemoveFeed = async (f: Feed) => {
    if (await confirm({ title: t("trends.removeFeedTitle"), body: t("trends.removeFeedBody", { name: f.title || f.url }), confirmLabel: t("common.delete"), tone: "danger" })) {
      removeFeed.mutate(f.id);
    }
  };

  const quota = integrations.data?.youtubeDataApi.quota;
  const watchErr = addWatch.error;
  const feedErr = addFeed.error;

  return (
    <>
      <div className="page-header">
        <div>
          <h1>{t("trends.title")}</h1>
          <p className="muted">{t("trends.subtitle")}</p>
        </div>
        <Button variant="primary" icon={RefreshCw} loading={refreshing} onClick={() => refresh.mutate()}>
          {t("trends.refreshNow")}
        </Button>
      </div>

      {integrations.isSuccess && !integrations.data.youtubeDataApi.configured && <Notice tone="warning">{t("trends.youtubeMissing")}</Notice>}
      {refreshJob && job.data?.status === "failed" && <ErrorState error={new Error(job.data.error ?? "")} />}

      <Section
        title={t("trends.watchTitle")}
        description={t("trends.watchHelp")}
        actions={quota ? <span className="small muted tabular">{t("launchpad.quota", { used: quota.usedToday, limit: quota.limit })}</span> : undefined}
      >
        <form className="field-group" onSubmit={onAddWatch}>
          <label htmlFor="watch-query">{t("trends.addWatch")}</label>
          <div className="input-row">
            <input
              id="watch-query"
              value={query}
              maxLength={200}
              placeholder={t("launchpad.searchPlaceholder")}
              aria-invalid={watchErr ? true : undefined}
              aria-describedby={watchErr ? "watch-error" : undefined}
              onChange={(e) => setQuery(e.target.value)}
            />
            <input id="watch-region" aria-label={t("niche.geo")} value={region} maxLength={2} style={{ width: 72 }} onChange={(e) => setRegion(e.target.value.toUpperCase())} />
            <Button type="submit" icon={Plus} loading={addWatch.isPending} disabled={!query.trim()}>
              {t("trends.watch")}
            </Button>
          </div>
          {watchErr && (
            <span id="watch-error" className="error-text" role="alert">
              {errorMessage(watchErr)}
            </span>
          )}
        </form>

        {watchlist.isPending && <Skeleton height={120} />}
        {watchlist.isError && <ErrorState error={watchlist.error} onRetry={() => void watchlist.refetch()} />}
        {watchlist.isSuccess && watchlist.data.length === 0 && (
          <Empty icon={Eye}>
            <p>{t("trends.watchEmpty")}</p>
          </Empty>
        )}
        {watchlist.isSuccess && watchlist.data.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">{t("trends.col.query")}</th>
                  <th scope="col">{t("trends.col.source")}</th>
                  <th scope="col">{t("trends.col.region")}</th>
                  <th scope="col">{t("trends.col.measured")}</th>
                  <th scope="col">{t("trends.col.next")}</th>
                  <th scope="col">
                    <span className="sr-only">{t("trends.col.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {watchlist.data.map((w) => (
                  <WatchRow key={w.id} watch={w} removing={removeWatch.isPending} onRemove={() => void confirmRemoveWatch(w)} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section title={t("trends.feedsTitle")} description={t("trends.feedsHelp")}>
        <form className="field-group" onSubmit={onAddFeed}>
          <label htmlFor="feed-url">{t("trends.addFeed")}</label>
          <div className="input-row">
            <input
              id="feed-url"
              type="url"
              inputMode="url"
              value={feedUrl}
              maxLength={2048}
              placeholder="https://example.org/feed.xml"
              aria-invalid={feedErr ? true : undefined}
              aria-describedby={feedErr ? "feed-help feed-error" : "feed-help"}
              onChange={(e) => setFeedUrl(e.target.value)}
            />
            <Button type="submit" icon={Rss} loading={addFeed.isPending} disabled={!feedUrl.trim()}>
              {t("trends.addFeedButton")}
            </Button>
          </div>
          <span id="feed-help" className="helper">
            {t("trends.feedHelper")}
          </span>
          {feedErr && (
            <span id="feed-error" className="error-text" role="alert">
              {errorMessage(feedErr)}
            </span>
          )}
        </form>
        {feeds.isPending && <Skeleton height={60} />}
        {feeds.isError && <ErrorState error={feeds.error} onRetry={() => void feeds.refetch()} />}
        {feeds.isSuccess && feeds.data.length === 0 && (
          <Empty icon={Newspaper}>
            <p>{t("trends.feedsEmpty")}</p>
          </Empty>
        )}
        {feeds.isSuccess && feeds.data.length > 0 && (
          <ul className="plain-list">
            {feeds.data.map((f) => (
              <FeedItem key={f.id} feed={f} onDelete={() => void confirmRemoveFeed(f)} />
            ))}
          </ul>
        )}
      </Section>

      <p className="xs muted row">
        <ChartNoAxesColumn size={14} aria-hidden="true" />
        {t("trends.footnote")}
      </p>
    </>
  );
}
