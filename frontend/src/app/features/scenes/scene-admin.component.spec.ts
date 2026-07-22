import { ComponentFixture, TestBed } from '@angular/core/testing';
import { SceneAdminComponent } from './scene-admin.component';
import { LiveState, Position, Scene } from '../../core/models';

function makePositions(): Position[] {
  return [
    { id: 'pos1', name: 'Principal', cameraId: 'cam1', isAudioSource: false },
    { id: 'pos2', name: 'Canto', cameraId: '', isAudioSource: false },
  ];
}

function makeScene(overrides: Partial<Scene> = {}): Scene {
  return { id: 'scene1', name: 'Cena 1', positionIds: [], ...overrides };
}

describe('SceneAdminComponent', () => {
  let fixture: ComponentFixture<SceneAdminComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [SceneAdminComponent] });
    fixture = TestBed.createComponent(SceneAdminComponent);
  });

  function setInputs(scenes: Scene[], liveState: LiveState | null = null): void {
    fixture.componentRef.setInput('scenes', scenes);
    fixture.componentRef.setInput('positions', makePositions());
    fixture.componentRef.setInput('liveState', liveState);
    fixture.detectChanges();
  }

  describe('creating a scene', () => {
    it('emits create with the name and selected position ids', () => {
      setInputs([]);

      let created: { name: string; positionIds: string[] } | undefined;
      fixture.componentInstance.create.subscribe((e: { name: string; positionIds: string[] }) => (created = e));

      const nameInput = fixture.nativeElement.querySelector(
        '.scene-admin__create-form .scene-admin__input',
      ) as HTMLInputElement;
      nameInput.value = 'Palco';
      nameInput.dispatchEvent(new Event('input'));

      const checkboxes = fixture.nativeElement.querySelectorAll(
        '.scene-admin__create-form input[type="checkbox"]',
      ) as NodeListOf<HTMLInputElement>;
      checkboxes[0].checked = true;
      checkboxes[0].dispatchEvent(new Event('change'));
      fixture.detectChanges();

      const form = fixture.nativeElement.querySelector('.scene-admin__create-form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));

      expect(created).toEqual({ name: 'Palco', positionIds: ['pos1'] });
    });

    it('shows a validation message and emits nothing when the name is empty', () => {
      setInputs([]);

      let created: unknown;
      fixture.componentInstance.create.subscribe((e: unknown) => (created = e));

      const form = fixture.nativeElement.querySelector('.scene-admin__create-form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));
      fixture.detectChanges();

      expect(created).toBeUndefined();
      expect(fixture.nativeElement.querySelector('.scene-admin__error')?.textContent).toContain('obrigatório');
    });
  });

  describe('live and preview badges', () => {
    it('renders a distinct "ao vivo" badge for the scene marked live', () => {
      const live: LiveState = { previewKind: '', previewId: '', liveKind: 'scene', liveId: 'scene1' };
      setInputs([makeScene({ id: 'scene1', name: 'Cena 1' })], live);

      const badge = fixture.nativeElement.querySelector('.badge--live');
      expect(badge).toBeTruthy();
      expect(badge.textContent?.trim()).toBe('ao vivo');
      expect(fixture.componentInstance.isLive(makeScene({ id: 'scene1' }))).toBe(true);
    });

    it('renders a distinct "em prévia" badge for the scene marked preview', () => {
      const live: LiveState = { previewKind: 'scene', previewId: 'scene2', liveKind: '', liveId: '' };
      setInputs([makeScene({ id: 'scene2', name: 'Cena 2' })], live);

      const badge = fixture.nativeElement.querySelector('.badge--preview');
      expect(badge).toBeTruthy();
      expect(badge.textContent?.trim()).toBe('em prévia');
    });
  });

  describe('deleting a scene', () => {
    it('still emits delete when clicking delete on the live scene (soft UI guard; 409 handled by parent)', () => {
      const live: LiveState = { previewKind: '', previewId: '', liveKind: 'scene', liveId: 'scene1' };
      setInputs([makeScene({ id: 'scene1', name: 'Cena 1' })], live);

      let deleted: string | undefined;
      fixture.componentInstance.delete.subscribe((id: string) => (deleted = id));

      const btn = fixture.nativeElement.querySelector('.btn--delete') as HTMLButtonElement;
      expect(btn.disabled).toBe(false);
      btn.click();

      expect(deleted).toBe('scene1');
    });
  });

  describe('editing positions', () => {
    it('emits updatePositions with the edited position set', () => {
      setInputs([makeScene({ id: 'scene1', positionIds: ['pos1'] })]);

      let updated: { id: string; positionIds: string[] } | undefined;
      fixture.componentInstance.updatePositions.subscribe(
        (e: { id: string; positionIds: string[] }) => (updated = e),
      );

      fixture.componentInstance.startEditPositions(makeScene({ id: 'scene1', positionIds: ['pos1'] }));
      fixture.detectChanges();

      const checkboxes = fixture.nativeElement.querySelectorAll(
        '.scene-admin__positions-form input[type="checkbox"]',
      ) as NodeListOf<HTMLInputElement>;
      // pos2 currently unchecked -> check it
      checkboxes[1].checked = true;
      checkboxes[1].dispatchEvent(new Event('change'));
      fixture.detectChanges();

      const form = fixture.nativeElement.querySelector('.scene-admin__positions-form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));

      expect(updated).toEqual({ id: 'scene1', positionIds: ['pos1', 'pos2'] });
    });
  });
});
