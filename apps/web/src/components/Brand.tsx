import { Play } from "lucide-react";

export function Brand() {
  return (
    <>
      <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary text-primary-foreground">
        <Play className="size-4 fill-current" />
      </span>
      <span className="text-lg font-semibold tracking-tight">
        GCFeed<span className="ml-0.5 text-muted-foreground">.</span>
      </span>
    </>
  );
}
