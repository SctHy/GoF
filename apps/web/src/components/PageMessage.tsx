import { Icon } from "@/components/Icon";
import { Button } from "@/components/ui/button";
import type { MessageProps } from "@/types";

export function PageMessage({ icon, title, action, onAction }: MessageProps) {
  return (
    <div
      className="flex min-h-52 flex-col items-center justify-center gap-4 rounded-xl border border-dashed px-6 py-12 text-center"
      role="status"
    >
      <span className="flex size-12 items-center justify-center rounded-2xl border bg-muted/50 text-muted-foreground">
        <Icon name={icon} />
      </span>
      <p className="max-w-sm text-sm text-muted-foreground">{title}</p>
      {action && (
        <Button variant="outline" onClick={onAction}>
          {action}
        </Button>
      )}
    </div>
  );
}
