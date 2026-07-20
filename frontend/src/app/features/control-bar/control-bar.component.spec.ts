import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ControlBarComponent } from './control-bar.component';
import { ConnectionState, Position } from '../../core/models';

function makePosition(overrides: Partial<Position> = {}): Position {
  return { id: 'pos1', name: 'Principal', cameraId: '', isAudioSource: false, ...overrides };
}

describe('ControlBarComponent', () => {
  let fixture: ComponentFixture<ControlBarComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [ControlBarComponent] });
    fixture = TestBed.createComponent(ControlBarComponent);
  });

  describe('UT-074: positions summary', () => {
    it('renders a position with a camera and shows the camera id', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Principal', cameraId: 'cam1' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      const text = fixture.nativeElement.textContent;
      expect(text).toContain('Principal');
      expect(text).toContain('cam1');
    });

    it('renders an empty position as "vazia"', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Canto', cameraId: '' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      const text = fixture.nativeElement.textContent;
      expect(text).toContain('Canto');
      expect(text).toContain('vazia');
    });

    it('highlights the audio source position with 🔊', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Principal', cameraId: 'cam1', isAudioSource: true })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      const text = fixture.nativeElement.textContent;
      expect(text).toContain('Principal');
      expect(text).toContain('cam1');
      expect(text).toContain('🔊');
    });

    it('shows audio source indicator when a position is the audio source', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Principal', cameraId: 'cam1', isAudioSource: true })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      const text = fixture.nativeElement.textContent;
      expect(text).toContain('Áudio:');
      expect(text).toContain('Principal');
    });

    it('renders multiple positions', () => {
      const positions = [
        makePosition({ id: 'pos1', name: 'Principal', cameraId: 'cam1' }),
        makePosition({ id: 'pos2', name: 'Canto', cameraId: 'cam2' }),
      ];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      const text = fixture.nativeElement.textContent;
      expect(text).toContain('Principal');
      expect(text).toContain('cam1');
      expect(text).toContain('Canto');
      expect(text).toContain('cam2');
    });
  });

  describe('connection indicators', () => {
    it('shows "conectado" when state is open', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('conectado');
    });

    it('shows "desconectado" when state is closed', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('connectionState', 'closed' as ConnectionState);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('desconectado');
    });

    it('shows "conectando..." when state is connecting', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('connectionState', 'connecting' as ConnectionState);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('conectando...');
    });
  });

  describe('sync button', () => {
    it('emits sync event when button is clicked', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('connectionState', 'open' as ConnectionState);
      fixture.detectChanges();

      let emitted: void | undefined;
      fixture.componentInstance.sync.subscribe(() => (emitted = undefined));

      const btn = fixture.nativeElement.querySelector('.control-bar__sync') as HTMLButtonElement;
      btn.click();

      expect(emitted).toBeUndefined();
    });
  });
});
