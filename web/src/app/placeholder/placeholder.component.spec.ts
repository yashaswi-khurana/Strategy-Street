import { TestBed } from '@angular/core/testing';
import { PlaceholderComponent } from './placeholder.component';

describe('PlaceholderComponent', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [PlaceholderComponent],
    }).compileComponents();
  });

  it('should create placeholder component', () => {
    const fixture = TestBed.createComponent(PlaceholderComponent);
    const comp = fixture.componentInstance;
    expect(comp).toBeTruthy();
  });

  it('should render header text', () => {
    const fixture = TestBed.createComponent(PlaceholderComponent);
    fixture.detectChanges();
    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.querySelector('h1')?.textContent).toContain('Hello, Strategy Street');
  });
});
