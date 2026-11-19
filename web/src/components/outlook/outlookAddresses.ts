export function outlookAddresses(value: string): string[] {
  return value.split(/[,;\s]+/).map((part) => part.trim()).filter(Boolean);
}
