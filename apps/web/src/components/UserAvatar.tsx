import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

export function UserAvatar({
  src,
  name,
  className,
}: {
  src?: string;
  name?: string;
  className?: string;
}) {
  return (
    <Avatar className={cn("size-9 shrink-0 text-sm", className)}>
      <AvatarImage src={src || undefined} alt="" />
      <AvatarFallback className="rounded-[inherit] text-[length:inherit]">
        {(name || "G").slice(0, 1).toUpperCase()}
      </AvatarFallback>
    </Avatar>
  );
}
