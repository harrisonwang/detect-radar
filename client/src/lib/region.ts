export function countryName(code: string): string {
  try {
    return new Intl.DisplayNames(['zh-CN'], { type: 'region' }).of(code) || code;
  } catch {
    return code;
  }
}
