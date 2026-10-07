import { ApiError, MalformedResponseError, ValidationError } from "@aurarisk/shared";

/** Turns a failed API call into a short message people can act on. */
export function describeError(err: unknown): string {
  if (err instanceof ValidationError) return err.message;
  if (err instanceof ApiError) {
    if (err.status === 429) return "Too many requests. Please wait a moment and try again.";
    if (err.status >= 500) return "The AuraRisk server had a problem. Please try again shortly.";
    return err.message;
  }
  if (err instanceof MalformedResponseError) return "The server sent an unexpected response.";
  // fetch() rejects with a TypeError when the network or CORS blocks the request.
  if (err instanceof TypeError) return "Can't reach the AuraRisk server. Check your connection.";
  return "Something went wrong. Please try again.";
}
