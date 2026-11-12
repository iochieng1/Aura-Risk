import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  backoffDelayMs,
  CORRUPT_QUEUE_KEY,
  countQueue,
  dequeueReport,
  discardQueuedReport,
  editQueuedReport,
  enqueueReport,
  getReportQueue,
  isDue,
  markReportCreated,
  markReportFailed,
  recordTransientFailure,
  removeQueuedPhoto,
  retryQueuedReport,
  RETRY_MAX_DELAY_MS,
  subscribeToReportQueue,
} from '../reportQueue';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

const report = {
  location: { name: 'Kibera bridge', lat: -1.29, lon: 36.82 },
  category: 'flooding' as const,
  note: 'Water rising near bridge',
};

describe('Mobile Report Queue Storage', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
  });

  it('should enqueue an offline report successfully', async () => {
    const queued = await enqueueReport(report);
    const queue = await getReportQueue();

    expect(queue).toHaveLength(1);
    expect(queue[0].tempId).toBe(queued.tempId);
    expect(queue[0].note).toBe('Water rising near bridge');
    expect(queue[0].location.name).toBe('Kibera bridge');
    expect(queue[0].photos).toEqual([]);
    expect(queue[0].serverReportId).toBeUndefined();
  });

  it('should dequeue a report when processed', async () => {
    const queued = await enqueueReport({ ...report, category: 'road_blocked', note: 'Tree down' });
    await dequeueReport(queued.tempId);

    expect(await getReportQueue()).toHaveLength(0);
  });

  it('stores photo uris and a server id for partially sent reports', async () => {
    const queued = await enqueueReport(report, ['file:///doc/a.jpg', 'file:///doc/b.jpg'], 'srv-1');
    const [item] = await getReportQueue();

    expect(item.tempId).toBe(queued.tempId);
    expect(item.photos).toEqual(['file:///doc/a.jpg', 'file:///doc/b.jpg']);
    expect(item.serverReportId).toBe('srv-1');
  });

  it('records the server id and removes uploaded photos one by one', async () => {
    const queued = await enqueueReport(report, ['file:///doc/a.jpg', 'file:///doc/b.jpg']);

    await markReportCreated(queued.tempId, 'srv-2');
    await removeQueuedPhoto(queued.tempId, 'file:///doc/a.jpg');

    const [item] = await getReportQueue();
    expect(item.serverReportId).toBe('srv-2');
    expect(item.photos).toEqual(['file:///doc/b.jpg']);
  });

  it('reads items queued before photo support', async () => {
    await AsyncStorage.setItem(
      '@aurarisk/offline_reports_queue',
      JSON.stringify([{ ...report, tempId: 'old', retryCount: 0, createdAt: '2026-01-01T00:00:00Z' }])
    );

    const [item] = await getReportQueue();
    expect(item.photos).toEqual([]);
  });

  it('migrates items stored before #80 without dropping any, even past the old retry limit', async () => {
    await AsyncStorage.setItem(
      '@aurarisk/offline_reports_queue',
      JSON.stringify([
        { ...report, tempId: 'a', retryCount: 0, createdAt: '2026-01-01T00:00:00Z', photos: [] },
        { ...report, tempId: 'b', retryCount: 7, createdAt: '2026-01-01T00:00:00Z', photos: ['file:///doc/b.jpg'] },
      ])
    );

    const queue = await getReportQueue();
    expect(queue.map((q) => [q.tempId, q.status, q.attempts])).toEqual([
      ['a', 'pending', 0],
      ['b', 'pending', 7],
    ]);
    expect(queue[1]).not.toHaveProperty('retryCount');
    expect(queue.every((q) => isDue(q))).toBe(true);
  });

  it('keeps a copy of an unreadable queue before anything overwrites it', async () => {
    jest.spyOn(console, 'error').mockImplementation(() => undefined);
    await AsyncStorage.setItem('@aurarisk/offline_reports_queue', '[{"tempId": "x", ');

    expect(await getReportQueue()).toEqual([]);
    await enqueueReport(report);

    expect(await AsyncStorage.getItem(CORRUPT_QUEUE_KEY)).toBe('[{"tempId": "x", ');
  });

  it('keeps a copy of a stored queue that is valid JSON but not a list', async () => {
    jest.spyOn(console, 'error').mockImplementation(() => undefined);
    await AsyncStorage.setItem('@aurarisk/offline_reports_queue', '{"tempId": "x"}');

    expect(await getReportQueue()).toEqual([]);
    await enqueueReport(report);

    expect(await AsyncStorage.getItem(CORRUPT_QUEUE_KEY)).toBe('{"tempId": "x"}');
  });

  it('backs off exponentially, capped at an hour', () => {
    expect([1, 2, 3, 4, 5, 6, 7, 8, 20].map(backoffDelayMs)).toEqual([
      30_000, 60_000, 120_000, 240_000, 480_000, 960_000, 1_920_000, RETRY_MAX_DELAY_MS, RETRY_MAX_DELAY_MS,
    ]);
  });

  it('schedules the next attempt after a transient failure and honours Retry-After', async () => {
    const queued = await enqueueReport(report);
    await recordTransientFailure(queued.tempId, 'network', 1_000);
    let [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'pending', attempts: 1, nextAttemptAt: 31_000, lastError: 'network' });
    expect(isDue(item, 30_999)).toBe(false);
    expect(isDue(item, 31_000)).toBe(true);

    await recordTransientFailure(queued.tempId, 'rate limited', 1_000, 300_000);
    [item] = await getReportQueue();
    expect(item).toMatchObject({ attempts: 2, nextAttemptAt: 301_000 });
  });

  it('keeps a rejected report as failed until the user retries, edits or discards it', async () => {
    const queued = await enqueueReport(report, ['file:///doc/a.jpg']);
    await markReportFailed(queued.tempId, 'invalid category');

    let [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'failed', lastError: 'invalid category' });
    expect(isDue(item, Number.MAX_SAFE_INTEGER)).toBe(false);

    await retryQueuedReport(queued.tempId);
    [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'pending', attempts: 0 });
    expect(item.lastError).toBeUndefined();
    expect(isDue(item)).toBe(true);

    await markReportFailed(queued.tempId, 'note is too long');
    await editQueuedReport(queued.tempId, { note: 'Shorter note' });
    [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'pending', note: 'Shorter note' });

    expect(await discardQueuedReport(queued.tempId)).toEqual(['file:///doc/a.jpg']);
    expect(await getReportQueue()).toEqual([]);
  });

  it('does not lose a report enqueued while another change is in progress', async () => {
    const first = await enqueueReport(report);
    await Promise.all([
      recordTransientFailure(first.tempId, 'network'),
      enqueueReport({ ...report, note: 'second' }),
      markReportCreated(first.tempId, 'srv-1'),
      enqueueReport({ ...report, note: 'third' }),
    ]);

    const queue = await getReportQueue();
    expect(queue.map((q) => q.note)).toEqual(['Water rising near bridge', 'second', 'third']);
    expect(queue[0]).toMatchObject({ attempts: 1, serverReportId: 'srv-1' });
  });

  it('tells subscribers about every change, and counts pending and failed reports', async () => {
    const seen: number[] = [];
    const unsubscribe = subscribeToReportQueue((queue) => seen.push(queue.length));
    const a = await enqueueReport(report);
    await enqueueReport(report);
    await markReportFailed(a.tempId, 'bad');
    unsubscribe();
    await dequeueReport(a.tempId);

    expect(seen).toEqual([1, 2, 2]);
    expect(countQueue(await getReportQueue())).toEqual({ pending: 1, failed: 0 });
  });
});
