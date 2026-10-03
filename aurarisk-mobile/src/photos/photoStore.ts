import { Directory, File, Paths } from 'expo-file-system';
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';
import * as ImagePicker from 'expo-image-picker';

export const MAX_PHOTOS_PER_REPORT = 4;
export const MAX_PHOTO_BYTES = 10 * 1024 * 1024;
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

const prepareAll = async (result: ImagePicker.ImagePickerResult): Promise<string[]> => {
  if (result.canceled || !result.assets) return [];
  const prepared: string[] = [];
  for (const asset of result.assets) {
    prepared.push(await preparePhoto(asset));
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
