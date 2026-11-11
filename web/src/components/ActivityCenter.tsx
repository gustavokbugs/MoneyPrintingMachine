import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Job } from "../api/types";
import { formatDateTime, t, type MessageKey } from "../i18n";
import { activityLink, isActive, kindLabelKey } from "./activity";
import { useToast } from "./feedback";

const SEEN_KEY = "activity.seenAt";

function readSeen(): number {
  try {
    return Number(localStorage.getItem(SEEN_KEY) ?? "0") || 0;
  } catch {
    return 0;
  }
}

function writeSeen(ms: number) {
  try {
    localStorage.setItem(SEEN_KEY, String(ms));
  } catch {
    // Storage may be unavailable; the unread count then resets on reload.
  }
}

function notificationsSupported(): boolean {
  return typeof window !== "undefined" && "Notification" in window;
}

/**
 * ActivityCenter lists background AI work (jobs run on the server, so they
 * keep going when you switch pages or tabs) and announces when each one
 * finishes: a toast in the app and, when allowed, a desktop notification
 * while the tab is hidden.
 */
export function ActivityCenter() {
  const toast = useToast();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [seenAt, setSeenAt] = useState(readSeen);
  const [permission, setPermission] = useState(() => (notificationsSupported() ? Notification.permission : "denied"));
  const jobs = useQuery({
    queryKey: ["activity"],
    queryFn: () => api.recentJobs(),
    refetchInterval: (q) => ((q.state.data ?? []).some(isActive) ? 4000 : 15000),
    refetchIntervalInBackground: true,
  });

  // Announce jobs that finished since the previous poll.
  const previous = useRef<Map<string, Job["status"]> | null>(null);
  useEffect(() => {
    const items = jobs.data;
    if (!items) return;
    const prev = previous.current;
    previous.current = new Map(items.map((j) => [j.id, j.status]));
    if (prev === null) return; // first load: nothing is "new"
    for (const j of items) {
      const before = prev.get(j.id);
      if (before === undefined || !(before === "queued" || before === "running") || isActive(j)) continue;
      const label = t(kindLabelKey(j.kind));
      const message = j.status === "succeeded" ? t("activity.finished", { task: label }) : t("activity.failedToast", { task: label });
      if (j.status === "succeeded") toast.success(message);
      else toast.error(message);
      if (document.hidden && permission === "granted") {
        try {
          new Notification(t("app.name"), { body: message, tag: j.id });
        } catch {
          // Some browsers only allow notifications from a service worker.
        }
      }
      // Pages showing this job's data refresh themselves.
      void qc.invalidateQueries({ queryKey: ["job", j.id] });
      if (j.channelId) void qc.invalidateQueries({ queryKey: ["channel", j.channelId] });
    }
  }, [jobs.data, toast, qc, permission]);

  const items = jobs.data ?? [];
  const active = items.filter(isActive).length;
  const unread = items.filter((j) => !isActive(j) && Date.parse(j.updatedAt) > seenAt).length;

  const toggle = () => {
    setOpen((o) => !o);
    if (!open) {
      const now = Date.now();
      setSeenAt(now);
      writeSeen(now);
    }
  };

  const enableDesktop = async () => {
    if (!notificationsSupported()) return;
    setPermission(await Notification.requestPermission());
  };

  return (
    <div className="activity">
      <button type="button" className="btn btn-ghost activity-toggle" aria-expanded={open} aria-controls="activity-panel" onClick={toggle}>
        {active > 0 ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : <Bell size={18} aria-hidden="true" />}
        <span>{t("activity.title")}</span>
        {(active > 0 || unread > 0) && (
          <span className="activity-badge" aria-label={t("activity.badge", { active, unread })}>
            {active > 0 ? active : unread}
          </span>
        )}
      </button>
      {open && (
        <div id="activity-panel" className="activity-panel card" role="region" aria-label={t("activity.title")}>
          <p className="small muted">{t("activity.help")}</p>
          {notificationsSupported() && permission === "default" && (
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => void enableDesktop()}>
              {t("activity.enableDesktop")}
            </button>
          )}
          {jobs.isError && <p className="small">{t("activity.loadError")}</p>}
          {items.length === 0 && !jobs.isError && <p className="small muted">{t("activity.empty")}</p>}
          <ul className="activity-list">
            {items.map((j) => (
              <li key={j.id} className="activity-item">
                {isActive(j) ? (
                  <LoaderCircle size={16} className="spin" aria-hidden="true" />
                ) : j.status === "succeeded" ? (
                  <CircleCheck size={16} aria-hidden="true" />
                ) : (
                  <CircleAlert size={16} aria-hidden="true" />
                )}
                <div className="activity-body">
                  <Link to={activityLink(j)} onClick={() => setOpen(false)}>
                    {t(kindLabelKey(j.kind))}
                  </Link>
                  <span className="xs muted">
                    {t(`activity.status.${j.status}` as MessageKey)} · {formatDateTime(j.updatedAt)}
                  </span>
                  {j.status === "failed" && j.error && <span className="xs">{j.error}</span>}
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
