import type { Message } from "../types.ts";

export function appendMessages<T extends { id: number }>(currentItems: T[], nextItems: T[]) {
  if (!nextItems.length) return currentItems;
  const seen = new Set(currentItems.map((item) => item.id));
  const merged = [...currentItems];
  for (const item of nextItems) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    merged.push(item);
  }
  return merged;
}

export function messageActor(message: Message) {
  if (!message.actor_id) return null;
  return {
    id: message.actor_id,
    nickname: message.actor_nickname || `用户_${message.actor_id}`,
    avatar_url: message.actor_avatar_url || ""
  };
}

export function messageBody(message: Message) {
  return message.content.trim();
}

export function messageIcon(type: string) {
  switch (type) {
    case "LIKE":
      return "favorite";
    case "COMMENT":
      return "chat_bubble";
    case "FOLLOW":
      return "person_add";
    case "SYSTEM":
      return "campaign";
    default:
      return "notifications";
  }
}
