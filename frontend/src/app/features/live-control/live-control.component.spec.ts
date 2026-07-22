import { ComponentFixture, TestBed } from '@angular/core/testing';
import { LiveControlComponent } from './live-control.component';
import { Camera, LiveKind, LiveState, Scene } from '../../core/models';

function makeCamera(overrides: Partial<Camera> = {}): Camera {
  return {
    id: 'camera1',
    name: 'Câmera 1',
    sourceUrl: 'rtmp://mediamtx:1935/camera1',
    status: 'online',
    lastSeenAt: '2026-07-16T14:00:00Z',
    ...overrides,
  };
}

describe('LiveControlComponent', () => {
  let fixture: ComponentFixture<LiveControlComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [LiveControlComponent] });
    fixture = TestBed.createComponent(LiveControlComponent);
  });

  function setInputs(liveState: LiveState | null): void {
    fixture.componentRef.setInput('cameras', [makeCamera({ id: 'camera1', name: 'Câmera 1' })]);
    const scenes: Scene[] = [{ id: 'scene1', name: 'Cena 1', positionIds: ['pos1'] }];
    fixture.componentRef.setInput('scenes', scenes);
    fixture.componentRef.setInput('liveState', liveState);
    fixture.detectChanges();
  }

  const emptyState: LiveState = { previewKind: '', previewId: '', liveKind: '', liveId: '' };

  describe('preview selection', () => {
    it('emits a camera preview-selection without emitting a cut', () => {
      setInputs(emptyState);

      let preview: { kind: LiveKind; id: string } | undefined;
      let cutEmitted = false;
      fixture.componentInstance.previewSelect.subscribe((e: { kind: LiveKind; id: string }) => (preview = e));
      fixture.componentInstance.cut.subscribe(() => (cutEmitted = true));

      const cameraBtn = fixture.nativeElement.querySelectorAll('.live-control__option')[0] as HTMLButtonElement;
      cameraBtn.click();

      expect(preview).toEqual({ kind: 'camera', id: 'camera1' });
      expect(cutEmitted).toBe(false);
    });

    it('emits a scene preview-selection without emitting a cut', () => {
      setInputs(emptyState);

      let preview: { kind: LiveKind; id: string } | undefined;
      let cutEmitted = false;
      fixture.componentInstance.previewSelect.subscribe((e: { kind: LiveKind; id: string }) => (preview = e));
      fixture.componentInstance.cut.subscribe(() => (cutEmitted = true));

      // second bus (Cenas) first option
      const sceneBtn = fixture.nativeElement.querySelectorAll(
        '.live-control__bus',
      )[1].querySelector('.live-control__option') as HTMLButtonElement;
      sceneBtn.click();

      expect(preview).toEqual({ kind: 'scene', id: 'scene1' });
      expect(cutEmitted).toBe(false);
    });
  });

  describe('cut action', () => {
    it('emits cut when there is an active preview', () => {
      const state: LiveState = { previewKind: 'camera', previewId: 'camera1', liveKind: '', liveId: '' };
      setInputs(state);

      let cutEmitted = false;
      fixture.componentInstance.cut.subscribe(() => (cutEmitted = true));

      const cutBtn = fixture.nativeElement.querySelector('.live-control__cut') as HTMLButtonElement;
      expect(cutBtn.disabled).toBe(false);
      cutBtn.click();

      expect(cutEmitted).toBe(true);
    });

    it('disables the cut button and emits nothing when no preview is selected (no direct-to-air path)', () => {
      setInputs(emptyState);

      let cutEmitted = false;
      fixture.componentInstance.cut.subscribe(() => (cutEmitted = true));

      const cutBtn = fixture.nativeElement.querySelector('.live-control__cut') as HTMLButtonElement;
      expect(cutBtn.disabled).toBe(true);
      fixture.componentInstance.onCut();

      expect(cutEmitted).toBe(false);
    });
  });

  describe('indicators', () => {
    it('shows the on-air source distinctly from the preview source', () => {
      const state: LiveState = {
        previewKind: 'scene',
        previewId: 'scene1',
        liveKind: 'camera',
        liveId: 'camera1',
      };
      setInputs(state);

      const onAir = fixture.nativeElement.querySelector('.live-control__indicator--onair');
      const preview = fixture.nativeElement.querySelector('.live-control__indicator--preview');
      expect(onAir.textContent).toContain('Câmera 1');
      expect(preview.textContent).toContain('Cena 1');
    });
  });
});
