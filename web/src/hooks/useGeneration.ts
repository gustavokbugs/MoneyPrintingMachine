import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, newIdempotencyKey } from "../api/client";
import { useToast } from "../components/feedback";
import { t, type MessageKey } from "../i18n";
import { useInvalidateChannel, useJob } from "./useJob";

export type GenerationStep = "strategy" | "identity" | "blueprint";

/**
 * useGeneration starts a channel generation job, refreshes the channel when
 * it finishes, and confirms completion with a toast.
 */
export function useGeneration(channelId: string, step: GenerationStep) {
  const [jobId, setJobId] = useState<string | null>(null);
  const invalidate = useInvalidateChannel(channelId);
  const toast = useToast();
  const start = useMutation({
    mutationFn: () => api.generate(channelId, step, newIdempotencyKey()),
    onSuccess: (j) => setJobId(j.id),
  });
  const job = useJob(jobId, () => {
    invalidate();
    toast.success(t(`generation.done.${step}` as MessageKey));
  });
  const running = start.isPending || job.data?.status === "queued" || job.data?.status === "running";
  return { start: () => start.mutate(), startError: start.error, job, jobId, running };
}
