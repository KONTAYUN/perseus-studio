export function cn(...parts) {
  return parts.flat().filter(Boolean).join(' ');
}
