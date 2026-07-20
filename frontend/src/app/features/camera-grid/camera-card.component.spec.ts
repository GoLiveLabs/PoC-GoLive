import { ComponentFixture, TestBed, fakeAsync, tick } from '@angular/core/testing';
import { CameraCardComponent } from './camera-card.component';
import { Camera, Position } from '../../core/models';

function makeCamera(overrides: Partial<Camera> = {}): Camera {
  return {
    id: 'camera1',
    name: 'camera1',
    sourceUrl: 'rtmp://mediamtx:1935/camera1',
    status: 'online',
    lastSeenAt: '2026-07-16T14:00:00Z',
    ...overrides,
  };
}

function makePositions(overrides: Partial<Position>[] = []): Position[] {
  return [
    { id: 'pos1', name: 'Principal', cameraId: '', isAudioSource: false, ...overrides[0] },
    { id: 'pos2', name: 'Canto', cameraId: '', isAudioSource: false, ...overrides[1] },
  ];
}

describe('CameraCardComponent', () => {
  let fixture: ComponentFixture<CameraCardComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [CameraCardComponent] });
    fixture = TestBed.createComponent(CameraCardComponent);
  });

  describe('position assignment control', () => {
    it('UT-069: renders 3 options (Nenhuma + 2 positions) when 2 positions exist', () => {
      fixture.componentRef.setInput('camera', makeCamera());
      fixture.componentRef.setInput('positions', makePositions());
      fixture.detectChanges();

      const options = fixture.nativeElement.querySelectorAll('option');
      expect(options.length).toBe(3);
      expect(options[0].textContent?.trim()).toBe('Nenhuma');
      expect(options[1].textContent?.trim()).toBe('Principal');
      expect(options[2].textContent?.trim()).toBe('Canto');
    });

    it('UT-070: pre-selects the position that matches the camera id', () => {
      const positions = makePositions([{ id: 'pos1', cameraId: 'camera1' }]);
      fixture.componentRef.setInput('camera', makeCamera({ id: 'camera1' }));
      fixture.componentRef.setInput('positions', positions);
      fixture.detectChanges();
      fixture.detectChanges();

      expect(fixture.componentInstance.currentPositionId()).toBe('pos1');
      const options = fixture.nativeElement.querySelectorAll('option');
      const selectedOption = Array.from(options).find((o) => (o as HTMLOptionElement).selected) as HTMLOptionElement | undefined;
      expect(selectedOption?.value).toBe('pos1');
    });

    it('UT-070: pre-selects Nenhuma when camera is not assigned to any position', () => {
      fixture.componentRef.setInput('camera', makeCamera({ id: 'camera1' }));
      fixture.componentRef.setInput('positions', makePositions());
      fixture.detectChanges();

      const select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
      expect(select.value).toBe('');
    });

    it('UT-071: selecting a position emits assign with positionId and cameraId', () => {
      fixture.componentRef.setInput('camera', makeCamera({ id: 'camera2', status: 'online' }));
      fixture.componentRef.setInput('positions', makePositions());
      fixture.detectChanges();

      let emitted: { positionId: string; cameraId: string } | undefined;
      fixture.componentInstance.assign.subscribe((e: { positionId: string; cameraId: string }) => (emitted = e));

      const select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
      select.value = 'pos1';
      select.dispatchEvent(new Event('change'));
      fixture.detectChanges();

      expect(emitted).toEqual({ positionId: 'pos1', cameraId: 'camera2' });
    });

    it('UT-071: selecting Nenhuma when assigned emits unassign', () => {
      const positions = makePositions([{ id: 'pos1', cameraId: 'camera1' }]);
      fixture.componentRef.setInput('camera', makeCamera({ id: 'camera1' }));
      fixture.componentRef.setInput('positions', positions);
      fixture.detectChanges();

      let unassigned: string | undefined;
      fixture.componentInstance.unassign.subscribe((id: string) => (unassigned = id));

      const select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
      select.value = '';
      select.dispatchEvent(new Event('change'));
      fixture.detectChanges();

      expect(unassigned).toBe('pos1');
    });

    it('UT-076: renaming a position (via positions signal update) updates the displayed name without changing selection', () => {
      const positions = makePositions([{ id: 'pos1', name: 'Principal', cameraId: 'camera1' }]);
      fixture.componentRef.setInput('camera', makeCamera({ id: 'camera1' }));
      fixture.componentRef.setInput('positions', positions);
      fixture.detectChanges();
      fixture.detectChanges();

      const options1 = fixture.nativeElement.querySelectorAll('option');
      const selectedOption1 = Array.from(options1).find((o) => (o as HTMLOptionElement).selected) as HTMLOptionElement | undefined;
      expect(selectedOption1?.value).toBe('pos1');
      expect(options1[1].textContent?.trim()).toBe('Principal');

      const updatedPositions = [{ id: 'pos1', name: 'Principal Renovado', cameraId: 'camera1', isAudioSource: false }];
      fixture.componentRef.setInput('positions', updatedPositions);
      fixture.detectChanges();
      fixture.detectChanges();

      const options2 = fixture.nativeElement.querySelectorAll('option');
      const selectedOption2 = Array.from(options2).find((o) => (o as HTMLOptionElement).selected) as HTMLOptionElement | undefined;
      expect(selectedOption2?.value).toBe('pos1');
      expect(options2[1].textContent?.trim()).toBe('Principal Renovado');
    });
  });

  describe('offline camera', () => {
    it('disables the select when camera is offline', () => {
      fixture.componentRef.setInput('camera', makeCamera({ status: 'offline' }));
      fixture.componentRef.setInput('positions', makePositions());
      fixture.detectChanges();

      const select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
      expect(select.disabled).toBe(true);
    });

    it('does not emit assign when selecting while disabled', () => {
      fixture.componentRef.setInput('camera', makeCamera({ status: 'offline' }));
      fixture.componentRef.setInput('positions', makePositions());
      fixture.detectChanges();

      let emitted: unknown;
      fixture.componentInstance.assign.subscribe((e: unknown) => (emitted = e));

      const select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
      select.value = 'pos1';
      select.dispatchEvent(new Event('change'));
      fixture.detectChanges();

      expect(emitted).toBeUndefined();
    });
  });

  describe('online badge', () => {
    it('shows online badge when camera is online', () => {
      fixture.componentRef.setInput('camera', makeCamera({ status: 'online' }));
      fixture.componentRef.setInput('positions', []);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('Online');
    });

    it('shows offline badge when camera is offline', () => {
      fixture.componentRef.setInput('camera', makeCamera({ status: 'offline' }));
      fixture.componentRef.setInput('positions', []);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('Offline');
    });
  });
});
