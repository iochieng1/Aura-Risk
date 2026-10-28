import { reportStatusLabel } from '../constants';

describe('reportStatusLabel', () => {
  it('shows the county response only for verified reports', () => {
    expect(reportStatusLabel({ status: 'pending', response_status: 'open' })).toBe('Unverified');
    expect(reportStatusLabel({})).toBe('Unverified');
    expect(reportStatusLabel({ status: 'verified' })).toBe('Verified · Awaiting county response');
    expect(reportStatusLabel({ status: 'verified', response_status: 'acknowledged' })).toBe('Verified · County acknowledged');
    expect(reportStatusLabel({ status: 'verified', response_status: 'resolved' })).toBe('Verified · County resolved');
  });
});
