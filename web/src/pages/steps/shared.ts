import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Channel } from "../../api/types";

/** useChannelUpdate runs a mutation that returns the updated channel and
 * writes it into the cache. */
export function useChannelUpdate<TArgs>(channelId: string, fn: (args: TArgs) => Promise<Channel>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (c) => {
      qc.setQueryData(["channel", channelId], c);
      void qc.invalidateQueries({ queryKey: ["channels"] });
    },
  });
}
