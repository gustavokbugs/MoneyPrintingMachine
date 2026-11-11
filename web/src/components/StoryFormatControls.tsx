import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { ProductionStatus, Video } from "../api/types";
import { t } from "../i18n";
import { errorMessage } from "./ui";
import { useToast } from "./feedback";

/**
 * StoryFormatControls picks how the next script tells the story: a calm
 * explainer or a short viral format (absurd escalation, POV story,
 * problem-fix chain).
 */
export function StoryFormatControls({ video: v, tools, disabled, onSaved }: { video: Video; tools: ProductionStatus | undefined; disabled: boolean; onSaved: (v: Video) => void }) {
  const toast = useToast();
  const set = useMutation({
    mutationFn: (format: string) => api.setStoryFormat(v.id, format),
    onSuccess: onSaved,
    onError: (err) => toast.error(errorMessage(err)),
  });
  const formats = tools?.storyFormats ?? [];
  if (formats.length === 0) return null;
  const current = formats.find((f) => f.id === v.storyFormat);
  return (
    <div className="stack-sm">
      <div className="row wrap">
        <label htmlFor="story-format" className="label">
          {t("video.storyFormat")}
        </label>
        <select id="story-format" value={v.storyFormat} disabled={disabled || set.isPending} onChange={(e) => set.mutate(e.target.value)}>
          {formats.map((f) => (
            <option key={f.id} value={f.id}>
              {f.name}
            </option>
          ))}
        </select>
      </div>
      {current?.example && <p className="xs muted">{current.example}</p>}
      {current && current.seconds > 0 && <p className="xs muted">{t("video.storyFormatHelp", { seconds: String(current.seconds) })}</p>}
    </div>
  );
}
