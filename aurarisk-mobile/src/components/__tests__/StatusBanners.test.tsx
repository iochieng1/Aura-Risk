import { fireEvent, render, screen } from '@testing-library/react-native';
import { I18nProvider } from '../../i18n';
import { markRequestSlow } from '../../net/serverStatus';
import StatusBanners from '../StatusBanners';

jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock')
);

const sync = { pendingCount: 0, failedCount: 0, isSyncing: false };
jest.mock('../../state/AppState', () => ({ useAppState: () => ({ sync }) }));

const renderBanners = (onReviewFailed?: () => void) =>
  render(
    <I18nProvider>
      <StatusBanners onReviewFailed={onReviewFailed} />
    </I18nProvider>
  );

beforeEach(() => {
  Object.assign(sync, { pendingCount: 0, failedCount: 0, isSyncing: false });
});

describe('StatusBanners', () => {
  it('renders nothing when there is nothing to report', () => {
    renderBanners();
    expect(screen.toJSON()).toBeNull();
  });

  it('shows queued and server-waking banners', () => {
    sync.pendingCount = 2;
    const settle = markRequestSlow();
    try {
      renderBanners();
      expect(screen.getByText('2 report(s) waiting to be sent')).toBeTruthy();
      expect(screen.getByText(/Connecting to the AuraRisk server/)).toBeTruthy();
    } finally {
      screen.unmount();
      settle();
    }
  });

  it('links failed reports to the review screen from other tabs', () => {
    sync.failedCount = 1;
    const onReviewFailed = jest.fn();
    renderBanners(onReviewFailed);
    fireEvent.press(screen.getByRole('button', { name: '1 report(s) not sent. Tap to review them.' }));
    expect(onReviewFailed).toHaveBeenCalled();
  });

  it('points at the list below on the screen that shows it', () => {
    sync.failedCount = 3;
    renderBanners();
    expect(screen.getByText('3 report(s) not sent. Check them below.')).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
