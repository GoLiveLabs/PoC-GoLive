import { provideHttpClient } from '@angular/common/http';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { App } from './app';
import { WebSocketService } from './core/websocket.service';

class FakeWebSocketService {
  cameras = signal<unknown[]>([]);
  systemStatus = signal<unknown>(null);
  connectionState = signal<'connecting' | 'open' | 'closed'>('closed');
  lastError = signal<string | null>(null);
  connect = () => {};
  disconnect = () => {};
}

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideHttpClient(), { provide: WebSocketService, useClass: FakeWebSocketService }],
    }).compileComponents();
  });

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
  });

  it('renders the control bar and camera grid', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('app-control-bar')).toBeTruthy();
    expect(compiled.querySelector('app-camera-grid')).toBeTruthy();
  });
});
