import { ComponentFixture, TestBed } from '@angular/core/testing';
import { BroadcastComponent } from './broadcast.component';
import { BroadcastStatus, Client, DestinationStatus } from '../../core/models';

function makeClients(): Client[] {
  return [
    { id: 'client1', name: 'Igreja A', createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-01T00:00:00Z' },
    { id: 'client2', name: 'Igreja B', createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-01T00:00:00Z' },
  ];
}

function makeStatus(overrides: Partial<BroadcastStatus> = {}): BroadcastStatus {
  return { activeClientId: null, running: false, destinations: [], ...overrides };
}

describe('BroadcastComponent', () => {
  let fixture: ComponentFixture<BroadcastComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [BroadcastComponent] });
    fixture = TestBed.createComponent(BroadcastComponent);
  });

  function setInputs(status: BroadcastStatus, onAir = false): void {
    fixture.componentRef.setInput('clients', makeClients());
    fixture.componentRef.setInput('status', status);
    fixture.componentRef.setInput('onAir', onAir);
    fixture.detectChanges();
  }

  describe('client selector', () => {
    it('renders one option per client plus the placeholder', () => {
      setInputs(makeStatus());

      const options = fixture.nativeElement.querySelectorAll('.broadcast__select option');
      expect(options.length).toBe(3);
    });

    it('emits set-active-client when a client is selected', () => {
      setInputs(makeStatus());

      let selected: string | undefined;
      fixture.componentInstance.setActiveClient.subscribe((id: string) => (selected = id));

      const select = fixture.nativeElement.querySelector('.broadcast__select') as HTMLSelectElement;
      select.value = 'client2';
      select.dispatchEvent(new Event('change'));

      expect(selected).toBe('client2');
    });
  });

  describe('start button enablement', () => {
    it('is disabled when no client is selected', () => {
      setInputs(makeStatus({ activeClientId: null }), true);
      const btn = fixture.nativeElement.querySelector('.btn--start') as HTMLButtonElement;
      expect(btn.disabled).toBe(true);
    });

    it('is disabled when nothing is on air', () => {
      setInputs(makeStatus({ activeClientId: 'client1' }), false);
      const btn = fixture.nativeElement.querySelector('.btn--start') as HTMLButtonElement;
      expect(btn.disabled).toBe(true);
    });

    it('is enabled when a client is active and something is on air', () => {
      setInputs(makeStatus({ activeClientId: 'client1' }), true);
      const btn = fixture.nativeElement.querySelector('.btn--start') as HTMLButtonElement;
      expect(btn.disabled).toBe(false);

      let started = false;
      fixture.componentInstance.start.subscribe(() => (started = true));
      btn.click();
      expect(started).toBe(true);
    });
  });

  describe('per-destination status', () => {
    it('renders each destination state and shows Reiniciar only for failed destinations', () => {
      const destinations: DestinationStatus[] = [
        { liveId: 'live1', platformName: 'YouTube', state: 'connected', lastError: '' },
        { liveId: 'live2', platformName: 'Twitch', state: 'failed', lastError: 'connection refused' },
      ];
      setInputs(makeStatus({ activeClientId: 'client1', running: true, destinations }), true);

      const items = fixture.nativeElement.querySelectorAll('.broadcast__destination');
      expect(items.length).toBe(2);
      expect(items[0].textContent).toContain('YouTube');
      expect(items[0].textContent).toContain('connected');
      expect(items[0].querySelector('.btn--restart')).toBeNull();

      expect(items[1].textContent).toContain('Twitch');
      expect(items[1].textContent).toContain('failed');
      expect(items[1].querySelector('.btn--restart')).toBeTruthy();
    });

    it('emits restart with the destination liveId when Reiniciar is clicked', () => {
      const destinations: DestinationStatus[] = [
        { liveId: 'live2', platformName: 'Twitch', state: 'failed', lastError: 'boom' },
      ];
      setInputs(makeStatus({ activeClientId: 'client1', running: true, destinations }), true);

      let restarted: string | undefined;
      fixture.componentInstance.restart.subscribe((id: string) => (restarted = id));

      const btn = fixture.nativeElement.querySelector('.btn--restart') as HTMLButtonElement;
      btn.click();

      expect(restarted).toBe('live2');
    });
  });

  describe('transmission state indicator', () => {
    it('shows "Transmitindo" when running and "Parada" otherwise', () => {
      setInputs(makeStatus({ running: true }));
      expect(fixture.nativeElement.querySelector('.broadcast__state').textContent).toContain('Transmitindo');

      setInputs(makeStatus({ running: false }));
      expect(fixture.nativeElement.querySelector('.broadcast__state').textContent).toContain('Parada');
    });
  });
});
