import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { api } from "../api/client";
import type { Job } from "../api/types";

/**
 * useJob polls a background job until it finishes. onSuccess runs once
 * when the job succeeds.
 */
export function useJob(jobId: string | null, onSuccess?: (job: Job) => void) {
  const query = useQuery({
    queryKey: ["job", jobId],
    queryFn: () => api.getJob(jobId as string),
    enabled: jobId !== null,
    refetchInterval: (q) => {
      const s = q.state.data?.status;
      return s === "succeeded" || s === "failed" ? false : 2000;
    },
  });
  const notified = useRef<string | null>(null);
  const job = query.data;
  useEffect(() => {
    if (job?.status === "succeeded" && notified.current !== job.id) {
      notified.current = job.id;
      onSuccess?.(job);
    }
  }, [job, onSuccess]);
  return query;
}

/** useInvalidateChannel returns a callback that refreshes channel data. */
export function useInvalidateChannel(channelId: string) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ["channel", channelId] });
    void qc.invalidateQueries({ queryKey: ["channels"] });
  };
}
