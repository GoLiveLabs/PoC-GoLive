import { Component, input, output, signal } from '@angular/core';
import { Camera, Position } from '../../core/models';

@Component({
  selector: 'app-position-admin',
  standalone: true,
  templateUrl: './position-admin.component.html',
  styleUrl: './position-admin.component.css',
})
export class PositionAdminComponent {
  positions = input.required<Position[]>();
  cameras = input.required<Camera[]>();

  create = output<{ name: string }>();
  rename = output<{ id: string; name: string }>();
  delete = output<string>();
  setAudio = output<string>();

  readonly newName = signal('');
  readonly validationError = signal<string | null>(null);
  readonly editingId = signal<string | null>(null);
  readonly editingName = signal('');

  onCreateSubmit(): void {
    const trimmed = this.newName().trim();
    if (!trimmed) {
      this.validationError.set('O nome da posição é obrigatório.');
      return;
    }
    this.validationError.set(null);
    this.create.emit({ name: trimmed });
    this.newName.set('');
  }

  startRename(position: Position): void {
    this.editingId.set(position.id);
    this.editingName.set(position.name);
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

  onDelete(id: string): void {
    this.delete.emit(id);
  }

  onSetAudio(positionId: string): void {
    this.setAudio.emit(positionId);
  }

  cameraName(cameraId: string): string {
    if (!cameraId) return '';
    const cam = this.cameras().find((c) => c.id === cameraId);
    return cam?.name ?? '';
  }
}
