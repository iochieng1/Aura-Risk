import { resolveRiskLink, riskAlertHref } from '../riskLink';
import type { RiskAlertData, Subscription } from '../../types/api';

const home: Subscription = {
  id: 'sub-1',
  name: 'Home',
  lat: -1.2921,
  lon: 36.8219,
  min_level: 'Alert',
  quiet_hours: null,
  allow_emergency_during_quiet_hours: false,
  last_notified_level: null,
  last_notified_at: null,
  created_at: '2026-11-01T00:00:00Z',
};

describe('riskAlertHref', () => {
  it('opens the Risk tab with the alert’s subscription and coordinates', () => {
    const alert: RiskAlertData = { type: 'risk_alert', subscription_id: 'sub-1', level: 'Alert', score: 70, lat: -1.29, lon: 36.82 };
    expect(riskAlertHref(alert)).toEqual({ pathname: '/', params: { subscription: 'sub-1', lat: '-1.29', lon: '36.82' } });
  });
});

describe('resolveRiskLink', () => {
  it('uses the followed location’s name with the link’s coordinates', () => {
    expect(resolveRiskLink({ subscription: 'sub-1', lat: '-1.3', lon: '36.8' }, [home], 'Alert location')).toEqual({
      location: { name: 'Home', lat: -1.3, lon: 36.8 },
      subscription: home,
    });
  });

  it('looks up coordinates from a subscription id alone', () => {
    expect(resolveRiskLink({ subscription: 'sub-1' }, [home], 'Alert location')?.location).toEqual({
      name: 'Home',
      lat: -1.2921,
      lon: 36.8219,
    });
  });

  it('falls back to a generic name when the subscription is no longer followed', () => {
    expect(resolveRiskLink({ subscription: 'gone', lat: '0.5', lon: '35' }, [home], 'Alert location')).toEqual({
      location: { name: 'Alert location', lat: 0.5, lon: 35 },
      subscription: undefined,
    });
  });

  it('takes the first value of repeated params', () => {
    expect(resolveRiskLink({ lat: ['1', '2'], lon: ['36', '37'] }, [], 'Pin')?.location).toEqual({ name: 'Pin', lat: 1, lon: 36 });
  });

  it('rejects links with nothing usable', () => {
    expect(resolveRiskLink({}, [home], 'Pin')).toBeNull();
    expect(resolveRiskLink({ subscription: 'unknown' }, [home], 'Pin')).toBeNull();
    expect(resolveRiskLink({ lat: 'abc', lon: '36' }, [], 'Pin')).toBeNull();
    expect(resolveRiskLink({ lat: '', lon: '36' }, [], 'Pin')).toBeNull();
    expect(resolveRiskLink({ lat: '95', lon: '36' }, [], 'Pin')).toBeNull();
  });
});
