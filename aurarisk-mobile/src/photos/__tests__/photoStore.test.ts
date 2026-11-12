import AsyncStorage from '@react-native-async-storage/async-storage';
import { enqueueReport, markReportFailed } from '../../storage/reportQueue';
import {
  MAX_STORED_PHOTO_BYTES,
  MAX_STORED_PHOTOS,
  ORPHAN_PHOTO_GRACE_MS,
  photoCapacity,
  pruneOrphanPhotos,
  storedPhotoUsage,
} from '../photoStore';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

// A tiny in-memory document directory.
jest.mock('expo-file-system', () => {
  const files = new Map<string, { size: number; modificationTime: number }>();
  const DIR = 'file:///doc/report-photos';
  class File {
    uri: string;
    constructor(uri: string) {
      this.uri = uri;
    }
    get name() {
      return this.uri.split('/').pop();
    }
    get exists() {
      return files.has(this.uri);
    }
    get size() {
      return files.get(this.uri)?.size ?? 0;
    }
    get modificationTime() {
      return files.get(this.uri)?.modificationTime ?? null;
    }
    delete() {
      files.delete(this.uri);
    }
  }
  class Directory {
    get exists() {
      return true;
    }
    list() {
      return [...files.keys()].map((uri) => new File(uri));
    }
  }
  return {
    File,
    Directory,
    Paths: { document: 'file:///doc' },
    __files: files,
    __add: (name: string, size: number, modificationTime: number) => files.set(`${DIR}/${name}`, { size, modificationTime }),
    __dir: DIR,
  };
});
jest.mock('expo-image-manipulator', () => ({}));
jest.mock('expo-image-picker', () => ({}));

const fs = jest.requireMock('expo-file-system');
const uri = (name: string) => `${fs.__dir}/${name}`;
const report = { location: { name: 'Bridge', lat: 0, lon: 0 }, category: 'flooding' as const, note: 'Water' };
const OLD = Date.now() - 2 * ORPHAN_PHOTO_GRACE_MS;

describe('photo storage', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    fs.__files.clear();
  });

  it('deletes old photos nothing references, but never those of a queued report', async () => {
    fs.__add('queued.jpg', 100, OLD);
    fs.__add('composer.jpg', 100, OLD);
    fs.__add('orphan-1.jpg', 100, OLD);
    fs.__add('orphan-2.jpg', 100, OLD + 1);
    fs.__add('just-attached.jpg', 100, Date.now());
    await enqueueReport(report, [uri('queued.jpg')]);

    expect(await pruneOrphanPhotos([uri('composer.jpg')])).toBe(2);

    expect([...fs.__files.keys()].sort()).toEqual([uri('composer.jpg'), uri('just-attached.jpg'), uri('queued.jpg')]);
  });

  it('protects photos of a failed (rejected) report too', async () => {
    fs.__add('rejected.jpg', 100, OLD);
    const queued = await enqueueReport(report, [uri('rejected.jpg')]);
    await markReportFailed(queued.tempId, 'bad');

    await pruneOrphanPhotos();

    expect(fs.__files.has(uri('rejected.jpg'))).toBe(true);
  });

  it('reports no room once the photo count or byte limit is reached by photos still needed', async () => {
    const photos: string[] = [];
    for (let i = 0; i < MAX_STORED_PHOTOS - 1; i++) {
      fs.__add(`p${i}.jpg`, 1000, OLD);
      photos.push(uri(`p${i}.jpg`));
    }
    await enqueueReport(report, photos);
    expect(await photoCapacity()).toBe(1);

    fs.__add('last.jpg', 1000, OLD);
    expect(await photoCapacity([uri('last.jpg')])).toBe(0);
    expect(storedPhotoUsage().count).toBe(MAX_STORED_PHOTOS);

    fs.__files.clear();
    fs.__add('huge.jpg', MAX_STORED_PHOTO_BYTES, OLD);
    await enqueueReport(report, [uri('huge.jpg')]);
    expect(await photoCapacity()).toBe(0);
  });

  it('frees room by deleting orphans first', async () => {
    for (let i = 0; i < MAX_STORED_PHOTOS; i++) fs.__add(`orphan${i}.jpg`, 1000, OLD);

    expect(await photoCapacity()).toBe(MAX_STORED_PHOTOS);
    expect(storedPhotoUsage()).toEqual({ count: 0, bytes: 0 });
  });
});
