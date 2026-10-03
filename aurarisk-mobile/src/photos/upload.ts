import { File, UploadType } from 'expo-file-system';
import { authFetch } from '../auth/authClient';
import { Photo, PhotoUpload } from '../types/api';
import { deleteLocalPhoto, MAX_PHOTO_BYTES } from './photoStore';

/** A failure that will not succeed on retry (file missing, too large, rejected by the server). */
export class PermanentPhotoError extends Error {}

const isPermanentStatus = (status: number) => status >= 400 && status < 500 && status !== 401 && status !== 408 && status !== 429;

/**
 * Upload flow from docs/MOBILE_PHASE3.md:
 *   1. POST /api/reports/:id/photos      -> presigned PUT URL into the quarantine prefix
 *   2. PUT bytes directly to object storage with the signed headers
 *   3. POST /api/photos/:id/complete      -> server scans, resizes, and publishes
 */
export const uploadReportPhoto = async (reportId: string, uri: string): Promise<Photo> => {
  const file = new File(uri);
  if (!file.exists) throw new PermanentPhotoError('Photo file no longer exists');
  const size = file.size ?? 0;
  if (size <= 0 || size > MAX_PHOTO_BYTES) throw new PermanentPhotoError(`Photo size ${size} is outside allowed range`);

  const slotResponse = await authFetch(`/api/reports/${encodeURIComponent(reportId)}/photos`, {
    method: 'POST',
    body: JSON.stringify({ content_type: 'image/jpeg', size_bytes: size }),
  });
  if (!slotResponse.ok) {
    const message = `Photo upload slot request failed (${slotResponse.status})`;
    throw isPermanentStatus(slotResponse.status) ? new PermanentPhotoError(message) : new Error(message);
  }
  const slot = (await slotResponse.json()) as PhotoUpload;

  const putResult = await file.upload(slot.upload_url, {
    httpMethod: slot.upload_method ?? 'PUT',
    uploadType: UploadType.BINARY_CONTENT,
    headers: slot.upload_headers,
    mimeType: 'image/jpeg',
  });
  if (putResult.status < 200 || putResult.status >= 300) {
    // A 403 from S3 usually means the presigned URL expired; a new slot on retry fixes that.
    throw new Error(`Object storage upload failed (${putResult.status})`);
  }

  const completeResponse = await authFetch(`/api/photos/${encodeURIComponent(slot.photo_id)}/complete`, {
    method: 'POST',
  });
  if (!completeResponse.ok) {
    throw new Error(`Photo completion failed (${completeResponse.status})`);
  }
  return (await completeResponse.json()) as Photo;
};

export const getPhotoStatus = async (photoId: string): Promise<Photo> => {
  const response = await authFetch(`/api/photos/${encodeURIComponent(photoId)}`);
  if (!response.ok) throw new Error(`Photo status request failed (${response.status})`);
  return (await response.json()) as Photo;
};

/**
 * Uploads photos one at a time. Each local copy is deleted (and `onDone` called) once the photo
 * is either uploaded or permanently rejected. Throws on the first retryable failure so the caller
 * can keep the remaining photos queued.
 */
export const uploadReportPhotos = async (
  reportId: string,
  uris: string[],
  onDone: (uri: string) => Promise<void> | void = () => undefined
): Promise<void> => {
  for (const uri of uris) {
    try {
      await uploadReportPhoto(reportId, uri);
    } catch (err) {
      if (!(err instanceof PermanentPhotoError)) throw err;
      console.warn('Dropping photo that cannot be uploaded:', err.message);
    }
    deleteLocalPhoto(uri);
    await onDone(uri);
  }
};
