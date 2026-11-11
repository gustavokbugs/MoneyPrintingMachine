import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Circle, Sparkles, Wand2 } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, newIdempotencyKey } from "../api/client";
import { asAutopilotResult } from "../api/guards";
import type { Channel } from "../api/types";
import { useJob } from "../hooks/useJob";
import { t, type MessageKey } from "../i18n";
import { Button, JobProgress, Notice, Section, errorMessage } from "./ui";
import { useToast } from "./feedback";

/** Steps the autopilot fills, in order, and how to tell they are done. */
const autoSteps: { id: string; done: (c: Channel) => boolean }[] = [
  { id: "niche", done: (c) => c.niche !== "" },
  { id: "strategy", done: (c) => c.strategy !== null },
  { id: "identity", done: (c) => c.identity !== null },
  { id: "name", done: (c) => c.name !== "" },
  { id: "blueprint", done: (c) => c.blueprint !== null },
];

/** needsAutopilot reports whether any automatic setup step is missing. */
export function needsAutopilot(c: Channel): boolean {
  return autoSteps.some((s) => !s.done(c));
}

/** useStartAutopilot creates a channel (optionally from a topic) and starts the autopilot job. */
export function useStartAutopilot() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  return useMutation({
    mutationFn: async (topic: string) => {
      const channel = await api.createChannel({ niche: topic.trim() });
      const job = await api.autopilot(channel.id, newIdempotencyKey());
      return { channel, job };
    },
    onSuccess: ({ channel, job }) => {
      void qc.invalidateQueries({ queryKey: ["channels"] });
      navigate(`/channels/${channel.id}?step=review&autopilot=${encodeURIComponent(job.id)}`);
    },
    onError: (err) => toast.error(errorMessage(err)),
  });
}

/** LaunchCard is the one-button channel setup on the Launchpad. */
export function LaunchCard() {
  const [topic, setTopic] = useState("");
  const start = useStartAutopilot();
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    start.mutate(topic);
  };
  return (
    <Section title={t("autopilot.launchTitle")} description={t("autopilot.launchHelp")}>
      <form onSubmit={onSubmit} className="field-group">
        <label htmlFor="autopilot-topic">{t("autopilot.topicLabel")}</label>
        <div className="input-row">
          <input
            id="autopilot-topic"
            value={topic}
            maxLength={120}
            placeholder={t("autopilot.topicPlaceholder")}
            onChange={(e) => setTopic(e.target.value)}
          />
          <Button type="submit" variant="primary" icon={Wand2} loading={start.isPending}>
            {t("autopilot.launch")}
          </Button>
        </div>
      </form>
    </Section>
  );
}

/**
 * AutopilotPanel shows the progress of an autopilot job on the channel page,
 * or offers to fill the missing steps automatically.
 */
export function AutopilotPanel({ channel, jobId, onStarted }: { channel: Channel; jobId: string | null; onStarted: (jobId: string) => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["channel", channel.id] });
    void qc.invalidateQueries({ queryKey: ["channels"] });
  };
  const job = useJob(jobId, refresh);
  const start = useMutation({
    mutationFn: () => api.autopilot(channel.id, newIdempotencyKey()),
    onSuccess: (j) => onStarted(j.id),
    onError: (err) => toast.error(errorMessage(err)),
  });
  const running = job.data?.status === "queued" || job.data?.status === "running";
  // While running, poll the channel so finished steps tick off live.
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => void qc.invalidateQueries({ queryKey: ["channel", channel.id] }), 3000);
    return () => clearInterval(timer);
  }, [running, qc, channel.id]);

  if (jobId === null) {
    if (!needsAutopilot(channel) || channel.status === "archived") return null;
    return (
      <Notice tone="info" title={t("autopilot.fillTitle")}>
        <p>{t("autopilot.fillHelp")}</p>
        <div>
          <Button variant="primary" size="sm" icon={Sparkles} loading={start.isPending} onClick={() => start.mutate()}>
            {t("autopilot.fill")}
          </Button>
        </div>
      </Notice>
    );
  }

  const result = job.data?.status === "succeeded" ? safeResult(job.data.result) : null;
  return (
    <Section title={t("autopilot.progressTitle")}>
      <ol className="stack-sm" aria-label={t("autopilot.progressTitle")}>
        {autoSteps.map((s) => {
          const done = s.done(channel);
          return (
            <li key={s.id} className="row">
              {done ? <Check size={16} aria-hidden="true" /> : <Circle size={16} aria-hidden="true" />}
              <span className={done ? "" : "muted"}>{t(`autopilot.step.${s.id}` as MessageKey)}</span>
              {done && <span className="sr-only">({t("wizard.stepDone")})</span>}
            </li>
          );
        })}
      </ol>
      <JobProgress job={job.data} error={job.error} />
      {result && (
        <Notice tone="success" title={t("autopilot.doneTitle")} live>
          {result.notes.map((n) => (
            <p key={n}>{n}</p>
          ))}
          <p>{t("autopilot.needsYou")}</p>
        </Notice>
      )}
    </Section>
  );
}

function safeResult(v: unknown) {
  try {
    return asAutopilotResult(v);
  } catch {
    return null;
  }
}
