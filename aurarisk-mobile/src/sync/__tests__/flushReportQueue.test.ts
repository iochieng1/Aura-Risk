import AsyncStorage from '@react-native-async-storage/async-storage';
import { enqueueReport, getReportQueue } from '../../storage/reportQueue';
import { flushReportQueue } from '../flushReportQueue';
import { createReport } from '../../services/api';
import { uploadReportPhotos } from '../../photos/upload';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

jest.mock('../../services/api', () => {
  class ReportRejectedError extends Error {}
  return {
    ReportRejectedError,
    isOnline: jest.fn(async () => true),
    createReport: jest.fn(),
  };
});

jest.mock('../../photos/upload', () => ({ uploadReportPhotos: jest.fn() }));
jest.mock('../../photos/photoStore', () => ({ deleteLocalPhoto: jest.fn() }));

const mockCreateReport = createReport as jest.MockedFunction<typeof createReport>;
const mockUpload = uploadReportPhotos as jest.MockedFunction<typeof uploadReportPhotos>;

const report = {
  location: { name: 'Bridge', lat: -1.29, lon: 36.82 },
  category: 'flooding' as const,
  note: 'Water rising',
};

describe('flushReportQueue', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    jest.clearAllMocks();
  });

  it('does not re-create a report when only its photos failed', async () => {
    await enqueueReport(report, ['file:///doc/a.jpg', 'file:///doc/b.jpg']);
    mockCreateReport.mockResolvedValue('srv-1');
    // First photo uploads, second fails with a retryable error.
    mockUpload.mockImplementationOnce(async (_id, _uris, onDone) => {
      await onDone?.('file:///doc/a.jpg');
      throw new Error('network');
    });

    const first = await flushReportQueue();
    expect(first).toEqual({ sent: 0, failed: 1, remaining: 1 });

    const [item] = await getReportQueue();
    expect(item.serverReportId).toBe('srv-1');
    expect(item.photos).toEqual(['file:///doc/b.jpg']);
    expect(item.retryCount).toBe(1);

    mockUpload.mockResolvedValueOnce(undefined);
    const second = await flushReportQueue();

    expect(second).toEqual({ sent: 1, failed: 0, remaining: 0 });
    expect(mockCreateReport).toHaveBeenCalledTimes(1);
    expect(mockUpload).toHaveBeenLastCalledWith('srv-1', ['file:///doc/b.jpg'], expect.any(Function));
  });

  it('shares one in-flight run between concurrent callers', async () => {
    await enqueueReport(report);
    mockCreateReport.mockResolvedValue('srv-2');
    mockUpload.mockResolvedValue(undefined);

    const [a, b] = await Promise.all([flushReportQueue(), flushReportQueue()]);

    expect(a).toBe(b);
    expect(mockCreateReport).toHaveBeenCalledTimes(1);
  });

  it('drops reports the server rejects as invalid', async () => {
    const { ReportRejectedError } = jest.requireMock('../../services/api');
    await enqueueReport(report, ['file:///doc/a.jpg']);
    mockCreateReport.mockRejectedValue(new ReportRejectedError('400'));

    const result = await flushReportQueue();

    expect(result.remaining).toBe(0);
    expect(jest.requireMock('../../photos/photoStore').deleteLocalPhoto).toHaveBeenCalledWith('file:///doc/a.jpg');
  });
});
