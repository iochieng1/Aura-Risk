import { Directory, File, Paths } from 'expo-file-system';
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';
import * as ImagePicker from 'expo-image-picker';
import { getReportQueue } from '../storage/reportQueue';

export const MAX_PHOTOS_PER_REPORT = 4;
export const MAX_PHOTO_BYTES = 10 * 1024 * 1024;

/**
 * Limits for photos kept on the device (docs/MOBILE_PHASE3.md, "Offline storage limits").
 * Attaching a photo is the only way one is stored, so it is refused once either limit is reached;
 * photos of queued (unsent) reports are never deleted to make room.
 */
export const MAX_STORED_PHOTOS = 40;
export const MAX_STORED_PHOTO_BYTES = 100 * 1024 * 1024;
/** A photo nothing references is only deleted once it is this old, so one being attached right now is safe. */
export const ORPHAN_PHOTO_GRACE_MS = 60 * 60_000;

/** Photo storage on the device is full; `prepared` holds the photos that were stored before it filled. */
export class PhotoStorageFullError extends Error {
  constructor(readonly prepared: string[] = []) {
    super('Photo storage is full');
    this.name = 'PhotoStorageFullError';
  }
}
const MAX_DIMENSION = 2048;
const JPEG_QUALITY = 0.8;

// Photos are kept in the document directory (not cache) so a report queued offline still has its
// photos after the OS evicts caches.
const photoDirectory = () => new Directory(Paths.document, 'report-photos');

const randomName = () => `${Date.now()}_${Math.random().toString(36).slice(2, 10)}.jpg`;

/**
 * Downscales to at most 2048px on the long edge and re-encodes as JPEG. Re-encoding drops EXIF
 * metadata (including GPS) before anything leaves the device. Returns the persisted file URI.
 */
export const preparePhoto = async (asset: { uri: string; width: number; height: number }): Promise<string> => {
  const context = ImageManipulator.manipulate(asset.uri);
  const longEdge = Math.max(asset.width, asset.height);
  if (longEdge > MAX_DIMENSION) {
    context.resize(asset.width >= asset.height ? { width: MAX_DIMENSION, height: null } : { width: null, height: MAX_DIMENSION });
  }
  const image = await context.renderAsync();
  const result = await image.saveAsync({ format: SaveFormat.JPEG, compress: JPEG_QUALITY });

  const dir = photoDirectory();
  dir.create({ intermediates: true, idempotent: true });
  const rendered = new File(result.uri);
  const destination = new File(dir, randomName());
  await rendered.copy(destination);
  try {
    rendered.delete();
  } catch {
    // The rendered copy lives in the cache directory; the OS will clean it up eventually.
  }
  return destination.uri;
};

export const deleteLocalPhoto = (uri: string): void => {
  try {
    const file = new File(uri);
    if (file.exists) file.delete();
  } catch (err) {
    console.warn('Failed to delete local photo:', err);
  }
};

export const localPhotoSize = (uri: string): number => new File(uri).size ?? 0;

const storedPhotos = (): File[] => {
  const dir = photoDirectory();
  if (!dir.exists) return [];
  return dir.list().filter((entry): entry is File => entry instanceof File);
};

const fileName = (uri: string) => uri.split('/').pop() ?? uri;

export interface PhotoUsage {
  count: number;
  bytes: number;
}

export const storedPhotoUsage = (): PhotoUsage =>
  storedPhotos().reduce((usage, file) => ({ count: usage.count + 1, bytes: usage.bytes + (file.size ?? 0) }), { count: 0, bytes: 0 });

/**
 * Deletes stored photos that no queued report references and that aren't in `keep` (e.g. photos
 * attached to the report being written), oldest first. Files younger than ORPHAN_PHOTO_GRACE_MS
 * are left alone. Returns how many were deleted.
 */
export const pruneOrphanPhotos = async (keep: string[] = [], now = Date.now()): Promise<number> => {
  const queue = await getReportQueue();
  const needed = new Set([...keep, ...queue.flatMap((item) => item.photos)].map(fileName));
  const orphans = storedPhotos()
    .filter((file) => !needed.has(file.name) && now - (file.modificationTime ?? now) >= ORPHAN_PHOTO_GRACE_MS)
    .sort((a, b) => (a.modificationTime ?? 0) - (b.modificationTime ?? 0));
  orphans.forEach((file) => deleteLocalPhoto(file.uri));
  return orphans.length;
};

/** How many more photos fit, after pruning orphans. 0 means storage is full. */
export const photoCapacity = async (keep: string[] = []): Promise<number> => {
  await pruneOrphanPhotos(keep);
  const usage = storedPhotoUsage();
  if (usage.bytes >= MAX_STORED_PHOTO_BYTES) return 0;
  return Math.max(0, MAX_STORED_PHOTOS - usage.count);
};

const prepareAll = async (result: ImagePicker.ImagePickerResult): Promise<string[]> => {
  if (result.canceled || !result.assets) return [];
  const prepared: string[] = [];
  for (const asset of result.assets) {
    const uri = await preparePhoto(asset);
    const usage = storedPhotoUsage();
    if (usage.count > MAX_STORED_PHOTOS || usage.bytes > MAX_STORED_PHOTO_BYTES) {
      deleteLocalPhoto(uri);
      throw new PhotoStorageFullError(prepared);
    }
    prepared.push(uri);
  }
  return prepared;
};

export const pickPhotosFromLibrary = async (remaining: number): Promise<string[]> => {
  if (remaining <= 0) return [];
  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ['images'],
    allowsMultipleSelection: remaining > 1,
    selectionLimit: remaining,
    quality: 1, // We compress once ourselves in preparePhoto.
    exif: false,
  });
  return (await prepareAll(result)).slice(0, remaining);
};

export const takePhoto = async (): Promise<string[] | 'permission_denied'> => {
  const permission = await ImagePicker.requestCameraPermissionsAsync();
  if (!permission.granted) return 'permission_denied';
  const result = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], quality: 1, exif: false });
  return prepareAll(result);
};
