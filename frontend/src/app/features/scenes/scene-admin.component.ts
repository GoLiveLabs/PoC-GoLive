import { Component, input, output, signal } from '@angular/core';
import { LiveState, Position, Scene } from '../../core/models';

@Component({
  selector: 'app-scene-admin',
  standalone: true,
  templateUrl: './scene-admin.component.html',
  styleUrl: './scene-admin.component.css',
})
export class SceneAdminComponent {
  scenes = input.required<Scene[]>();
  positions = input.required<Position[]>();
  liveState = input<LiveState | null>(null);

  create = output<{ name: string; positionIds: string[] }>();
  rename = output<{ id: string; name: string }>();
  updatePositions = output<{ id: string; positionIds: string[] }>();
  delete = output<string>();

  readonly newName = signal('');
  readonly newPositionIds = signal<string[]>([]);
  readonly validationError = signal<string | null>(null);

  readonly editingId = signal<string | null>(null);
  readonly editingName = signal('');

  readonly editingPositionsId = signal<string | null>(null);
  readonly editingPositionIds = signal<string[]>([]);

  isLive(scene: Scene): boolean {
    const state = this.liveState();
    return !!state && state.liveKind === 'scene' && state.liveId === scene.id;
  }

  isPreview(scene: Scene): boolean {
    const state = this.liveState();
    return !!state && state.previewKind === 'scene' && state.previewId === scene.id;
  }

  onCreateSubmit(): void {
    const trimmed = this.newName().trim();
    if (!trimmed) {
      this.validationError.set('O nome da cena é obrigatório.');
      return;
    }
    this.validationError.set(null);
    this.create.emit({ name: trimmed, positionIds: this.newPositionIds() });
    this.newName.set('');
    this.newPositionIds.set([]);
  }

  toggleNewPosition(positionId: string, checked: boolean): void {
    this.newPositionIds.update((ids) =>
      checked ? [...ids, positionId] : ids.filter((id) => id !== positionId),
    );
  }

  startRename(scene: Scene): void {
    this.editingId.set(scene.id);
    this.editingName.set(scene.name);
  }

  onRenameSubmit(): void {
    const id = this.editingId();
    if (!id) return;
    const trimmed = this.editingName().trim();
    if (!trimmed) {
      return;
    }
    this.rename.emit({ id, name: trimmed });
    this.editingId.set(null);
    this.editingName.set('');
  }

  cancelRename(): void {
    this.editingId.set(null);
    this.editingName.set('');
  }

  startEditPositions(scene: Scene): void {
    this.editingPositionsId.set(scene.id);
    this.editingPositionIds.set([...scene.positionIds]);
  }

  toggleEditPosition(positionId: string, checked: boolean): void {
    this.editingPositionIds.update((ids) =>
      checked ? [...ids, positionId] : ids.filter((id) => id !== positionId),
    );
  }

  onEditPositionsSubmit(): void {
    const id = this.editingPositionsId();
    if (!id) return;
    this.updatePositions.emit({ id, positionIds: this.editingPositionIds() });
    this.editingPositionsId.set(null);
    this.editingPositionIds.set([]);
  }

  cancelEditPositions(): void {
    this.editingPositionsId.set(null);
    this.editingPositionIds.set([]);
  }

  onDelete(id: string): void {
    this.delete.emit(id);
  }

  positionName(positionId: string): string {
    return this.positions().find((p) => p.id === positionId)?.name ?? positionId;
  }
}
