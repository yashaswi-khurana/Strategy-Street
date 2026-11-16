import { Routes } from '@angular/router';
import { PlaceholderComponent } from './placeholder/placeholder.component';

export const routes: Routes = [
  {
    path: '',
    component: PlaceholderComponent,
  },
  {
    path: 'placeholder',
    component: PlaceholderComponent,
  },
];
