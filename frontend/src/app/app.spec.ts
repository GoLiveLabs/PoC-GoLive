import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { App } from './app';
import { WebSocketService } from './core/websocket.service';

class FakeWebSocketService {
  cameras = signal<unknown[]>([]);
  positions = signal<unknown[]>([]);
  scenes = signal<unknown[]>([]);
  liveState = signal<unknown>(null);
  broadcastStatus = signal<unknown>(null);
  systemStatus = signal<unknown>(null);
  connectionState = signal<'connecting' | 'open' | 'closed'>('closed');
  lastError = signal<string | null>(null);
  connect = () => {};
  disconnect = () => {};
}

describe('App', () => {
  let httpMock: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: WebSocketService, useClass: FakeWebSocketService },
      ],
    }).compileComponents();
    httpMock = TestBed.inject(HttpTestingController);
  });

  function flushClients(): void {
    const req = httpMock.expectOne((r) => r.url.endsWith('/clients'));
    req.flush({ data: [], nextCursor: null, hasMore: false });
  }

  it('should create the app', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const app = fixture.componentInstance;
    expect(app).toBeTruthy();
    flushClients();
  });

  it('renders the control bar and camera grid', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('app-control-bar')).toBeTruthy();
    expect(compiled.querySelector('app-camera-grid')).toBeTruthy();
    flushClients();
  });

  it('wires the three new feature components (live-control, broadcast, scene-admin)', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('app-live-control')).toBeTruthy();
    expect(compiled.querySelector('app-broadcast')).toBeTruthy();
    expect(compiled.querySelector('app-scene-admin')).toBeTruthy();
    flushClients();
  });

  it('loads the client list into the clients signal on init', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    const req = httpMock.expectOne((r) => r.url.endsWith('/clients'));
    req.flush({
      data: [{ id: 'c1', name: 'Cliente 1', createdAt: '', updatedAt: '' }],
      nextCursor: null,
      hasMore: false,
    });
    expect((fixture.componentInstance as unknown as { clients: () => unknown[] }).clients().length).toBe(1);
  });

  afterEach(() => {
    httpMock.verify();
  });
});
