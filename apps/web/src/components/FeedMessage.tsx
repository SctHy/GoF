import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { PageMessage } from "@/components/PageMessage";
import type { MessageProps } from "@/types";

export function FeedMessage(props: MessageProps) {
  if (props.icon === "hourglass_top")
    return (
      <div className="w-full space-y-4 p-6" role="status">
        <Skeleton className="h-64 w-full rounded-xl" />
        <Skeleton className="h-5 w-2/5" />
        <p className="text-sm text-muted-foreground">{props.title}</p>
      </div>
    );
  return (
    <div className="w-full p-6">
      <PageMessage {...props} />
    </div>
  );
}
export function FeedPaginationNotice({
  error,
  onRefresh,
}: {
  error: string;
  onRefresh: () => void;
}) {
  if (!error) return null;
  return (
    <Button
      type="button"
      variant="secondary"
      className="feed-loading-pill h-auto whitespace-normal"
      onClick={onRefresh}
    >
      {error}
    </Button>
  );
}
