import { createReport, isPermanentRejection, ReportRejectedError } from '../api';
import { authFetch } from '../../auth/authClient';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);
jest.mock('@react-native-community/netinfo', () => ({ fetch: jest.fn() }));
jest.mock('../../auth/authClient', () => ({ authFetch: jest.fn() }));
jest.mock('../../photos/upload', () => ({ uploadReportPhotos: jest.fn() }));

const mockAuthFetch = authFetch as jest.MockedFunction<typeof authFetch>;
const report = { location: { name: 'Bridge', lat: -1.29, lon: 36.82 }, category: 'flooding' as const, note: 'Water rising' };
const reply = (status: number, body: unknown) =>
  ({ ok: status < 300, status, headers: new Headers(), json: async () => body }) as unknown as Response;

describe('createReport error classification', () => {
  it('treats 4xx as permanent except 401, 408 and 429', () => {
    expect([400, 403, 404, 409, 413, 422].every(isPermanentRejection)).toBe(true);
    expect([401, 408, 429, 500, 502, 503].some(isPermanentRejection)).toBe(false);
  });

  it('rejects with the server reason on a permanent 4xx', async () => {
    mockAuthFetch.mockResolvedValueOnce(reply(400, { error: 'invalid category' }));
    const err = await createReport(report).catch((e) => e);
    expect(err).toBeInstanceOf(ReportRejectedError);
    expect(err).toMatchObject({ message: 'invalid category', status: 400 });
  });

  it.each([408, 429, 503])('lets a %i through as a transient error', async (status) => {
    mockAuthFetch.mockResolvedValueOnce(reply(status, { error: 'later' }));
    const err = await createReport(report).catch((e) => e);
    expect(err).not.toBeInstanceOf(ReportRejectedError);
    expect(err.status).toBe(status);
  });
});
