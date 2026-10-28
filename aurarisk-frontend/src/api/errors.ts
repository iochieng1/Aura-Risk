import { ApiError, MalformedResponseError, translate, ValidationError, type Lang } from "@aurarisk/shared";

/** Turns a failed API call into a short message people can act on. */
export function describeError(err: unknown, lang: Lang = "en"): string {
  if (err instanceof ValidationError) return err.message;
  if (err instanceof ApiError) {
    if (err.status === 429) return translate(lang, "error.tooManyRequests");
    if (err.status >= 500) return translate(lang, "error.server");
    return err.message;
  }
  if (err instanceof MalformedResponseError) return translate(lang, "error.malformed");
  // fetch() rejects with a TypeError when the network or CORS blocks the request.
  if (err instanceof TypeError) return translate(lang, "error.network");
  return translate(lang, "error.generic");
}
