import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  dequeueReport,
  enqueueReport,
  getReportQueue,
  markReportCreated,
  removeQueuedPhoto,
  updateRetryCount,
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
    await updateRetryCount(queued.tempId);

    const [item] = await getReportQueue();
    expect(item.serverReportId).toBe('srv-2');
    expect(item.photos).toEqual(['file:///doc/b.jpg']);
    expect(item.retryCount).toBe(1);
  });

  it('reads items queued before photo support', async () => {
    await AsyncStorage.setItem(
      '@aurarisk/offline_reports_queue',
      JSON.stringify([{ ...report, tempId: 'old', retryCount: 0, createdAt: '2026-01-01T00:00:00Z' }])
    );

    const [item] = await getReportQueue();
    expect(item.photos).toEqual([]);
  });
});
