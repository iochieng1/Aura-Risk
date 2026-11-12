import AsyncStorage from '@react-native-async-storage/async-storage';
import { ApiError } from '@aurarisk/shared';
import { countQueue, enqueueReport, getReportQueue, RETRY_MAX_DELAY_MS } from '../../storage/reportQueue';
import { flushReportQueue } from '../flushReportQueue';
import { discardReport, editReport, retryReport } from '../queueActions';
import { createReport, isOnline } from '../../services/api';
import { uploadReportPhotos } from '../../photos/upload';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

jest.mock('../../services/api', () => {
  class ReportRejectedError extends Error {
    status?: number;
    constructor(message: string, status?: number) {
      super(message);
      this.status = status;
    }
  }
  class ReportValidationError extends ReportRejectedError {}
  return {
    ReportRejectedError,
    ReportValidationError,
    isOnline: jest.fn(async () => true),
    createReport: jest.fn(),
  };
});

jest.mock('../../photos/upload', () => ({ uploadReportPhotos: jest.fn() }));
jest.mock('../../photos/photoStore', () => ({ deleteLocalPhoto: jest.fn() }));

const mockCreateReport = createReport as jest.MockedFunction<typeof createReport>;
const mockUpload = uploadReportPhotos as jest.MockedFunction<typeof uploadReportPhotos>;
const mockIsOnline = isOnline as jest.MockedFunction<typeof isOnline>;
const mockDeletePhoto = jest.requireMock('../../photos/photoStore').deleteLocalPhoto as jest.Mock;
const { ReportRejectedError } = jest.requireMock('../../services/api');

const report = {
  location: { name: 'Bridge', lat: -1.29, lon: 36.82 },
  category: 'flooding' as const,
  note: 'Water rising',
};

let now = 1_000_000;

describe('flushReportQueue', () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    jest.clearAllMocks();
    mockIsOnline.mockResolvedValue(true);
    now = 1_000_000;
    jest.spyOn(Date, 'now').mockImplementation(() => now);
  });

  afterEach(() => {
    jest.restoreAllMocks();
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
    expect(first).toMatchObject({ sent: 0, rejected: 0, deferred: 1, remaining: 1 });

    const [item] = await getReportQueue();
    expect(item.serverReportId).toBe('srv-1');
    expect(item.photos).toEqual(['file:///doc/b.jpg']);
    expect(item.attempts).toBe(1);

    now = item.nextAttemptAt!;
    mockUpload.mockResolvedValueOnce(undefined);
    const second = await flushReportQueue();

    expect(second).toMatchObject({ sent: 1, deferred: 0, remaining: 0 });
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

  it('keeps a rejected report as failed, with the server reason and its photos, and does not retry it', async () => {
    await enqueueReport(report, ['file:///doc/a.jpg']);
    mockCreateReport.mockRejectedValue(new ReportRejectedError('invalid category', 400));

    const result = await flushReportQueue();

    expect(result).toMatchObject({ sent: 0, rejected: 1, remaining: 1 });
    expect(result.nextAttemptAt).toBeUndefined();
    const [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'failed', lastError: 'invalid category', photos: ['file:///doc/a.jpg'] });
    expect(mockDeletePhoto).not.toHaveBeenCalled();
    expect(countQueue(await getReportQueue())).toEqual({ pending: 0, failed: 1 });

    // Much later, automatic flushes still leave it alone.
    now += 7 * 24 * 60 * 60_000;
    await flushReportQueue();
    expect(mockCreateReport).toHaveBeenCalledTimes(1);
    expect(await getReportQueue()).toHaveLength(1);
  });

  it('sends a rejected report once the user edits it', async () => {
    const queued = await enqueueReport(report);
    mockCreateReport.mockRejectedValueOnce(new ReportRejectedError('note is too long', 400));
    await flushReportQueue();

    await editReport(queued.tempId, { category: 'road_blocked', note: 'Tree across the road' });
    mockCreateReport.mockResolvedValueOnce('srv-3');
    mockUpload.mockResolvedValue(undefined);
    const result = await flushReportQueue();

    expect(result).toMatchObject({ sent: 1, remaining: 0 });
    expect(mockCreateReport).toHaveBeenLastCalledWith(
      expect.objectContaining({ category: 'road_blocked', note: 'Tree across the road' })
    );
  });

  it('rejects an edit that fails validation and keeps the report as it was', async () => {
    const queued = await enqueueReport(report);
    mockCreateReport.mockRejectedValueOnce(new ReportRejectedError('bad', 400));
    await flushReportQueue();

    await expect(editReport(queued.tempId, { category: 'flooding', note: '   ' })).rejects.toBeInstanceOf(ReportRejectedError);
    const [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'failed', note: 'Water rising' });
  });

  it('removes a report and its photos only when the user discards it', async () => {
    const queued = await enqueueReport(report, ['file:///doc/a.jpg', 'file:///doc/b.jpg']);
    mockCreateReport.mockRejectedValueOnce(new ReportRejectedError('bad', 422));
    await flushReportQueue();
    expect(await getReportQueue()).toHaveLength(1);

    await discardReport(queued.tempId);

    expect(await getReportQueue()).toEqual([]);
    expect(mockDeletePhoto.mock.calls.map(([uri]) => uri)).toEqual(['file:///doc/a.jpg', 'file:///doc/b.jpg']);
  });

  it('keeps retrying through repeated network failures with growing delays, then recovers', async () => {
    await enqueueReport(report);
    mockCreateReport.mockRejectedValue(new TypeError('Network request failed'));

    const delays: number[] = [];
    for (let i = 0; i < 12; i++) {
      const result = await flushReportQueue();
      expect(result).toMatchObject({ sent: 0, deferred: 1, remaining: 1 });
      delays.push(result.nextAttemptAt! - now);

      // Not due yet: a flush in between doesn't touch it.
      now += 1;
      await flushReportQueue();
      now = result.nextAttemptAt!;
    }

    expect(mockCreateReport).toHaveBeenCalledTimes(12);
    expect(delays.slice(0, 4)).toEqual([30_000, 60_000, 120_000, 240_000]);
    expect(delays.at(-1)).toBe(RETRY_MAX_DELAY_MS);
    const [item] = await getReportQueue();
    expect(item).toMatchObject({ status: 'pending', attempts: 12, lastError: 'Network request failed' });

    mockCreateReport.mockResolvedValueOnce('srv-4');
    mockUpload.mockResolvedValue(undefined);
    const recovered = await flushReportQueue();
    expect(recovered).toMatchObject({ sent: 1, remaining: 0 });
    expect(recovered.nextAttemptAt).toBeUndefined();
  });

  it('treats 5xx, 408 and 429 as transient and waits as long as Retry-After asks', async () => {
    await enqueueReport(report);
    mockCreateReport.mockRejectedValueOnce(new ApiError(429, 'too many requests', 'too_many_requests', undefined, 600));

    const result = await flushReportQueue();

    expect(result).toMatchObject({ rejected: 0, deferred: 1, nextAttemptAt: now + 600_000 });
    for (const status of [408, 500, 503]) {
      now = (await getReportQueue())[0].nextAttemptAt!;
      mockCreateReport.mockRejectedValueOnce(new ApiError(status, 'nope'));
      expect(await flushReportQueue()).toMatchObject({ rejected: 0, deferred: 1 });
    }
    expect((await getReportQueue())[0].status).toBe('pending');
  });

  it('lets the user send a report now instead of waiting for its backoff', async () => {
    const queued = await enqueueReport(report);
    mockCreateReport.mockRejectedValueOnce(new Error('timeout'));
    await flushReportQueue();

    await retryReport(queued.tempId);
    mockCreateReport.mockResolvedValueOnce('srv-5');
    mockUpload.mockResolvedValue(undefined);

    expect(await flushReportQueue()).toMatchObject({ sent: 1, remaining: 0 });
  });

  it('stops without counting an attempt when the device goes offline', async () => {
    await enqueueReport(report);
    mockIsOnline.mockResolvedValue(false);

    const result = await flushReportQueue();

    expect(result).toMatchObject({ sent: 0, deferred: 0, remaining: 1 });
    expect(mockCreateReport).not.toHaveBeenCalled();
    expect((await getReportQueue())[0].attempts).toBe(0);
  });

  it('sends reports stored by an older version past the old retry limit instead of dropping them', async () => {
    await AsyncStorage.setItem(
      '@aurarisk/offline_reports_queue',
      JSON.stringify([{ ...report, tempId: 'old', retryCount: 3, createdAt: '2026-01-01T00:00:00Z', photos: [] }])
    );
    mockCreateReport.mockResolvedValueOnce('srv-6');
    mockUpload.mockResolvedValue(undefined);

    expect(await flushReportQueue()).toMatchObject({ sent: 1, remaining: 0 });
  });
});
