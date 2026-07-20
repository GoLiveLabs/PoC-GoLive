import { ComponentFixture, TestBed } from '@angular/core/testing';
import { PositionAdminComponent } from './position-admin.component';
import { Camera, Position } from '../../core/models';

function makePosition(overrides: Partial<Position> = {}): Position {
  return { id: 'pos1', name: 'Principal', cameraId: '', isAudioSource: false, ...overrides };
}

function makeCamera(overrides: Partial<Camera> = {}): Camera {
  return {
    id: 'cam1',
    name: 'camera1',
    sourceUrl: 'rtmp://mediamtx:1935/camera1',
    status: 'online',
    lastSeenAt: '2026-07-16T14:00:00Z',
    ...overrides,
  };
}

describe('PositionAdminComponent', () => {
  let fixture: ComponentFixture<PositionAdminComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [PositionAdminComponent] });
    fixture = TestBed.createComponent(PositionAdminComponent);
  });

  describe('empty state', () => {
    it('UT-075: renders empty-state message when positions array is empty', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      const el = fixture.nativeElement;
      expect(el.textContent).toContain('Nenhuma posição cadastrada');
      expect(el.querySelector('.position-admin__list')).toBeNull();
    });
  });

  describe('create', () => {
    it('UT-072: emits create with entered name on submit', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      let emitted: { name: string } | undefined;
      fixture.componentInstance.create.subscribe((e: { name: string }) => (emitted = e));

      const input = fixture.nativeElement.querySelector('.position-admin__input') as HTMLInputElement;
      input.value = 'Novo Nome';
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();

      const form = fixture.nativeElement.querySelector('.position-admin__create-form');
      form.dispatchEvent(new Event('submit'));
      fixture.detectChanges();

      expect(emitted).toEqual({ name: 'Novo Nome' });
    });

    it('UT-073: rejects empty name and shows validation message', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      let emitted: { name: string } | undefined;
      fixture.componentInstance.create.subscribe((e: { name: string }) => (emitted = e));

      const input = fixture.nativeElement.querySelector('.position-admin__input') as HTMLInputElement;
      input.value = '   ';
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();

      const form = fixture.nativeElement.querySelector('.position-admin__create-form');
      form.dispatchEvent(new Event('submit'));
      fixture.detectChanges();

      expect(emitted).toBeUndefined();
      expect(fixture.nativeElement.textContent).toContain('O nome da posição é obrigatório');
    });

    it('trims whitespace from name before emitting', () => {
      fixture.componentRef.setInput('positions', []);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      let emitted: { name: string } | undefined;
      fixture.componentInstance.create.subscribe((e: { name: string }) => (emitted = e));

      const input = fixture.nativeElement.querySelector('.position-admin__input') as HTMLInputElement;
      input.value = '  Espaçado  ';
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();

      const form = fixture.nativeElement.querySelector('.position-admin__create-form');
      form.dispatchEvent(new Event('submit'));
      fixture.detectChanges();

      expect(emitted).toEqual({ name: 'Espaçado' });
    });
  });

  describe('rename', () => {
    it('emits rename with id and new name on submit', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Antigo' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      let emitted: { id: string; name: string } | undefined;
      fixture.componentInstance.rename.subscribe((e: { id: string; name: string }) => (emitted = e));

      const renameBtn = fixture.nativeElement.querySelector('.btn--rename') as HTMLButtonElement;
      renameBtn.click();
      fixture.detectChanges();

      const renameInput = fixture.nativeElement.querySelector('.position-admin__rename-form input') as HTMLInputElement;
      renameInput.value = 'Novo Nome';
      renameInput.dispatchEvent(new Event('input'));
      fixture.detectChanges();

      const renameForm = fixture.nativeElement.querySelector('.position-admin__rename-form');
      renameForm.dispatchEvent(new Event('submit'));
      fixture.detectChanges();

      expect(emitted).toEqual({ id: 'pos1', name: 'Novo Nome' });
    });

    it('cancel rename clears editing state', () => {
      const positions = [makePosition({ id: 'pos1', name: 'Antigo' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      const renameBtn = fixture.nativeElement.querySelector('.btn--rename') as HTMLButtonElement;
      renameBtn.click();
      fixture.detectChanges();

      expect(fixture.componentInstance.editingId()).toBe('pos1');

      const cancelBtn = fixture.nativeElement.querySelector('.btn--cancel') as HTMLButtonElement;
      cancelBtn.click();
      fixture.detectChanges();

      expect(fixture.componentInstance.editingId()).toBeNull();
    });
  });

  describe('delete', () => {
    it('emits delete with position id', () => {
      const positions = [makePosition({ id: 'pos1' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      let emitted: string | undefined;
      fixture.componentInstance.delete.subscribe((id: string) => (emitted = id));

      const deleteBtn = fixture.nativeElement.querySelector('.btn--delete') as HTMLButtonElement;
      deleteBtn.click();
      fixture.detectChanges();

      expect(emitted).toBe('pos1');
    });
  });

  describe('set audio', () => {
    it('emits setAudio with position id', () => {
      const positions = [makePosition({ id: 'pos1', cameraId: 'cam1' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', [makeCamera()]);
      fixture.detectChanges();

      let emitted: string | undefined;
      fixture.componentInstance.setAudio.subscribe((id: string) => (emitted = id));

      const audioBtn = fixture.nativeElement.querySelector('.btn--audio') as HTMLButtonElement;
      audioBtn.click();
      fixture.detectChanges();

      expect(emitted).toBe('pos1');
    });
  });

  describe('list display', () => {
    it('shows camera name when position has a camera assigned', () => {
      const positions = [makePosition({ id: 'pos1', cameraId: 'cam1' })];
      const cameras = [makeCamera({ id: 'cam1', name: 'Câmera Principal' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', cameras);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('Câmera Principal');
    });

    it('shows empty indicator when position has no camera', () => {
      const positions = [makePosition({ id: 'pos1', cameraId: '' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', []);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('vazia');
    });

    it('shows audio badge when position is audio source', () => {
      const positions = [makePosition({ id: 'pos1', cameraId: 'cam1', isAudioSource: true })];
      const cameras = [makeCamera({ id: 'cam1' })];
      fixture.componentRef.setInput('positions', positions);
      fixture.componentRef.setInput('cameras', cameras);
      fixture.detectChanges();

      expect(fixture.nativeElement.textContent).toContain('🔊');
    });
  });
});
