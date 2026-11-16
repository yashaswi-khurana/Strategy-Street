import { Component } from '@angular/core';

@Component({
  selector: 'app-placeholder',
  standalone: true,
  template: `
    <main class="placeholder-container">
      <h1>Hello, Strategy Street</h1>
      <p>Algorithmic Trading & Deterministic Backtesting Platform</p>
      <span class="status-badge">System Initialized</span>
    </main>
  `,
  styles: [`
    .placeholder-container {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      font-family: system-ui, -apple-system, sans-serif;
      background-color: #0f172a;
      color: #f8fafc;
      text-align: center;
    }
    .status-badge {
      display: inline-block;
      margin-top: 1rem;
      padding: 0.25rem 0.75rem;
      border-radius: 9999px;
      background-color: #059669;
      color: white;
      font-size: 0.875rem;
      font-weight: 500;
    }
  `]
})
export class PlaceholderComponent {}
