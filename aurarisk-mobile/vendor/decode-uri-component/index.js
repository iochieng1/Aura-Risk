'use strict';

// A run of percent-encoded bytes, e.g. "%C3%A9%20". The pattern has no nested or overlapping
// repetition, so matching it is linear in the input length.
const ENCODED_RUN = /(?:%[0-9a-f]{2})+/gi;
const ENCODED_BYTE = /%[0-9a-f]{2}/gi;

/** Bytes in the UTF-8 sequence that starts with `byte`, or 0 if it cannot start one. */
const sequenceLength = (byte) => {
  if (byte < 0x80) return 1;
  if (byte >= 0xc2 && byte <= 0xdf) return 2;
  if (byte >= 0xe0 && byte <= 0xef) return 3;
  if (byte >= 0xf0 && byte <= 0xf4) return 4;
  return 0;
};

/** Whether bytes[start..start+length) is well-formed UTF-8 (RFC 3629: no overlongs or surrogates). */
const isWellFormed = (bytes, start, length) => {
  for (let k = 1; k < length; k += 1) {
    const b = bytes[start + k];
    let min = 0x80;
    let max = 0xbf;
    if (k === 1) {
      const lead = bytes[start];
      if (lead === 0xe0) min = 0xa0;
      else if (lead === 0xed) max = 0x9f;
      else if (lead === 0xf0) min = 0x90;
      else if (lead === 0xf4) max = 0x8f;
    }
    if (b < min || b > max) return false;
  }
  return true;
};

/**
 * Decodes what it can of a run that is not valid UTF-8 as a whole, one sequence at a time. Each
 * byte is looked at a bounded number of times and nothing throws, so this is linear too. Bytes that
 * are not part of a valid sequence are kept as written.
 */
const decodeRun = (run) => {
  try {
    return decodeURIComponent(run);
  } catch {
    // Fall through to sequence-by-sequence decoding.
  }
  const tokens = run.match(ENCODED_BYTE);
  const bytes = tokens.map((token) => parseInt(token.slice(1), 16));
  let out = '';
  let i = 0;
  while (i < bytes.length) {
    const length = sequenceLength(bytes[i]);
    if (length > 0 && i + length <= bytes.length && isWellFormed(bytes, i, length)) {
      out += decodeURIComponent(tokens.slice(i, i + length).join(''));
      i += length;
    } else {
      out += tokens[i];
      i += 1;
    }
  }
  return out;
};

/** Same contract as decode-uri-component: decodes as much as it can and never throws on bad input. */
module.exports = function decodeUriComponent(encodedURI) {
  if (typeof encodedURI !== 'string') {
    throw new TypeError('Expected `encodedURI` to be of type `string`, got `' + typeof encodedURI + '`');
  }
  try {
    return decodeURIComponent(encodedURI);
  } catch {
    return encodedURI.replace(ENCODED_RUN, decodeRun);
  }
};
