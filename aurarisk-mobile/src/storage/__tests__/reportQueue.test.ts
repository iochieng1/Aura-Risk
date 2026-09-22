import { enqueueReport, getReportQueue, dequeueReport } from '../reportQueue';
import AsyncStorage from '@react-native-async-storage/async-storage';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

describe('Mobile Report Queue Storage', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
  });

  it('should enqueue an offline report successfully', async () => {
    const mockReport = {
      location: { lat: -1.29, lon: 36.82 },
      category: 'flooding' as const,
      note: 'Water rising near bridge',
    };

    const queued = await enqueueReport(mockReport);
    const queue = await getReportQueue();

    expect(queue).toHaveLength(1);
    expect(queue[0].tempId).toBe(queued.tempId);
    expect(queue[0].note).toBe('Water rising near bridge');
  });

  it('should dequeue a report when processed', async () => {
    const mockReport = {
      location: { lat: -1.29, lon: 36.82 },
      category: 'road_blocked' as const,
      note: 'Tree down',
    };

    const queued = await enqueueReport(mockReport);
    await dequeueReport(queued.tempId);

    const queue = await getReportQueue();
    expect(queue).toHaveLength(0);
  });
});
