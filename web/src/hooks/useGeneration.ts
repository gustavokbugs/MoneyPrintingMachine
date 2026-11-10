import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, newIdempotencyKey } from "../api/client";
import { useInvalidateChannel, useJob } from "./useJob";

/**
 * useGeneration starts a channel generation job and refreshes the channel
 * when it finishes.
 */
export function useGeneration(channelId: string, step: "strategy" | "identity" | "blueprint") {
  const [jobId, setJobId] = useState<string | null>(null);
  const invalidate = useInvalidateChannel(channelId);
  const start = useMutation({
    mutationFn: () => api.generate(channelId, step, newIdempotencyKey()),
    onSuccess: (j) => setJobId(j.id),
  });
  const job = useJob(jobId, invalidate);
  const running = start.isPending || job.data?.status === "queued" || job.data?.status === "running";
  return { start: () => start.mutate(), startError: start.error, job, jobId, running };
}
