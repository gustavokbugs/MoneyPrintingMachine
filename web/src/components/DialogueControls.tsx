import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { DialogueModeInfo, ProductionStatus, Video } from "../api/types";
import { t } from "../i18n";
import { errorMessage } from "./ui";
import { useToast } from "./feedback";

/** speakerRole names who says a section in a two-voice script. */
export function speakerRole(modes: DialogueModeInfo[], v: Video, speaker: string | undefined): string {
  const mode = modes.find((m) => m.id === v.dialogueMode);
  if (!mode || mode.id === "narration") return "";
  return speaker === "B" ? mode.speakerB : mode.speakerA;
}

/**
 * DialogueControls picks one narrator or a two-voice format (Q&A, debate,
 * narrator and character) and the second voice. The format shapes the
 * next generated script.
 */
export function DialogueControls({ video: v, tools, disabled, onSaved }: { video: Video; tools: ProductionStatus | undefined; disabled: boolean; onSaved: (v: Video) => void }) {
  const toast = useToast();
  const set = useMutation({
    mutationFn: (input: { mode: string; voiceB: number }) => api.setDialogue(v.id, input.mode, input.voiceB),
    onSuccess: onSaved,
    onError: (err) => toast.error(errorMessage(err)),
  });
  const modes = tools?.dialogueModes ?? [];
  if (modes.length === 0) return null;
  const two = v.dialogueMode !== "narration";
  return (
    <div className="stack-sm">
      <div className="row wrap">
        <label htmlFor="dialogue-mode" className="label">
          {t("video.dialogue")}
        </label>
        <select id="dialogue-mode" value={v.dialogueMode} disabled={disabled || set.isPending} onChange={(e) => set.mutate({ mode: e.target.value, voiceB: v.voiceB })}>
          {modes.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
            </option>
          ))}
        </select>
        {two && (
          <>
            <label htmlFor="voice-b" className="label">
              {t("video.voiceB", { role: modes.find((m) => m.id === v.dialogueMode)?.speakerB ?? "" })}
            </label>
            <select id="voice-b" value={v.voiceB} disabled={disabled || set.isPending} onChange={(e) => set.mutate({ mode: v.dialogueMode, voiceB: Number(e.target.value) })}>
              {(tools?.voices ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          </>
        )}
      </div>
      <p className="xs muted">{t("video.dialogueHelp")}</p>
    </div>
  );
}
