export function isVideoSource(url: string | null | undefined) {
  return /\.(mp4|webm|ogg|mov)(\?|#|$)/i.test(url || "");
}
